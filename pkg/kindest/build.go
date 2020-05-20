package kindest

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Jeffail/tunny"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/term"
	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"github.com/jhoonb/archivex"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
)

type BuildOptions struct {
	File        string `json:"file,omitempty"`
	NoCache     bool   `json:"nocache,omitempty"`
	Squash      bool   `json:"squash,omitempty"`
	Tag         string `json:"tag,omitempty"`
	Concurrency int    `json:"concurrency"`
}

func buildDependencies(
	spec *KindestSpec,
	manifestPath string,
	options *BuildOptions,
	cli client.APIClient,
	pool *tunny.Pool,
) error {
	n := len(spec.Dependencies)
	dones := make([]chan error, n, n)
	rootDir := filepath.Dir(manifestPath)
	for i, dep := range spec.Dependencies {
		done := make(chan error, 1)
		dones[i] = done
		go func(dep string, done chan<- error) {
			opts := &BuildOptions{}
			*opts = *options
			opts.File = filepath.Clean(filepath.Join(rootDir, dep, "kindest.yaml"))
			err, _ := pool.Process(opts).(error)
			done <- err
			close(done)
		}(dep, done)
	}
	var multi error
	for i, done := range dones {
		if err := <-done; err != nil {
			multi = multierror.Append(multi, fmt.Errorf("dependency '%s': %v", spec.Dependencies[i], err))
		}
	}
	return multi
}

func locateSpec(options *BuildOptions) (string, error) {
	if options.File != "" {
		var err error
		file, err := filepath.Abs(options.File)
		if err != nil {
			return "", err
		}
		return file, nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "kindest.yaml"), nil
}

func resolveDockerfile(manifestPath string, spec *KindestSpec) (string, error) {
	rootDir := filepath.Dir(manifestPath)
	dockerfilePath := filepath.Clean(filepath.Join(rootDir, spec.Build.Docker.Dockerfile))
	contextPath := filepath.Clean(filepath.Join(rootDir, spec.Build.Docker.Context))
	dockerfileParts := strings.Split(dockerfilePath, "/")
	contextParts := strings.Split(contextPath, "/")
	var n int
	if m, o := len(dockerfileParts), len(contextParts); m < 0 {
		n = m
	} else {
		n = o
	}
	var common int
	for i := 0; i < n; i++ {
		if dockerfileParts[i] != contextParts[i] {
			break
		}
		common++
	}
	return filepath.Join(dockerfileParts[common:]...), nil
}

func loadSpec(options *BuildOptions) (*KindestSpec, string, error) {
	manifestPath, err := locateSpec(options)
	if err != nil {
		return nil, "", err
	}
	docBytes, err := ioutil.ReadFile(manifestPath)
	if err != nil {
		return nil, "", err
	}
	spec := &KindestSpec{}
	if err := yaml.Unmarshal(docBytes, spec); err != nil {
		return nil, "", err
	}
	if err := spec.Validate(manifestPath); err != nil {
		return nil, "", err
	}
	return spec, manifestPath, nil
}

func Build(options *BuildOptions, cli client.APIClient) error {
	var pool *tunny.Pool
	pool = tunny.NewFunc(runtime.NumCPU(), func(payload interface{}) interface{} {
		options := payload.(*BuildOptions)
		return BuildEx(options, cli, pool, nil)
	})
	defer pool.Close()
	return BuildEx(options, cli, pool, nil)
}

func BuildEx(
	options *BuildOptions,
	cli client.APIClient,
	pool *tunny.Pool,
	respHandler func(io.ReadCloser),
) error {
	spec, manifestPath, err := loadSpec(options)
	if err != nil {
		return err
	}
	log.Info("Loaded spec", zap.String("path", manifestPath))
	if err := buildDependencies(
		spec,
		manifestPath,
		options,
		cli,
		pool,
	); err != nil {
		return err
	}
	image := spec.Build.Name + ":" + options.Tag
	docker := spec.Build.Docker
	contextPath := filepath.Clean(filepath.Join(filepath.Dir(manifestPath), docker.Context))
	u, err := user.Current()
	if err != nil {
		return err
	}
	tmpDir := filepath.Join(u.HomeDir, ".kindest/tmp")
	if err := os.MkdirAll(tmpDir, 0766); err != nil {
		return err
	}
	ctxPath := filepath.Join(tmpDir, fmt.Sprintf("build-context-%s.tar", uuid.New().String()))
	log.Info("Building",
		zap.String("image", image),
		zap.String("context", contextPath),
		zap.String("tempdir", ctxPath),
		zap.Bool("noCache", options.NoCache))
	tar := new(archivex.TarFile)
	tar.Create(ctxPath)
	tar.AddAll(contextPath, false)
	tar.Close()
	defer os.Remove(ctxPath)
	dockerBuildContext, err := os.Open(ctxPath)
	if err != nil {
		return err
	}
	defer dockerBuildContext.Close()
	buildArgs := make(map[string]*string)
	for _, arg := range docker.BuildArgs {
		buildArgs[arg.Name] = &arg.Value
	}
	resolvedDockerfile, err := resolveDockerfile(
		manifestPath,
		spec,
	)
	if err != nil {
		return err
	}
	tag := fmt.Sprintf("%s:latest", spec.Build.Name)
	resp, err := cli.ImageBuild(
		context.TODO(),
		dockerBuildContext,
		types.ImageBuildOptions{
			NoCache:    options.NoCache,
			CacheFrom:  []string{tag},
			Dockerfile: resolvedDockerfile,
			BuildArgs:  buildArgs,
			Squash:     options.Squash,
			Tags:       []string{tag},
		},
	)
	if err != nil {
		return err
	}

	if respHandler != nil {
		respHandler(resp.Body)
	} else {
		termFd, isTerm := term.GetFdInfo(os.Stderr)
		if err := jsonmessage.DisplayJSONMessagesStream(
			resp.Body,
			os.Stderr,
			termFd,
			isTerm,
			nil,
		); err != nil {
			return err
		}
	}

	return nil
}
