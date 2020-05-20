package kindest

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

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
	File    string `json:"file,omitempty"`
	NoCache bool   `json:"nocache,omitempty"`
	Squash  bool   `json:"squash,omitempty"`
	Tag     string `json:"tag,omitempty"`
}

func buildDependencies(spec *KindestSpec, manifestPath string, options *BuildOptions, cli client.APIClient) error {
	n := len(spec.Dependencies)
	dones := make([]chan error, n, n)
	for i, dep := range spec.Dependencies {
		done := make(chan error, 1)
		dones[i] = done
		go func(dep string, done chan<- error) {
			opts := &BuildOptions{}
			*opts = *options
			opts.File = filepath.Join(manifestPath, dep, "kindest.yaml")
			done <- Build(opts, cli)
			close(done)
		}(dep, done)
	}
	var multi error
	for i, done := range dones {
		if err := <-done; err != nil {
			multi = multierror.Append(multi, fmt.Errorf("%s: %v", spec.Dependencies[i], err))
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
	spec, manifestPath, err := loadSpec(options)
	if err != nil {
		return err
	}
	if err := buildDependencies(
		spec,
		manifestPath,
		options,
		cli,
	); err != nil {
		return err
	}
	image := spec.Name + ":" + options.Tag
	log.Info("Building",
		zap.String("image", image),
		zap.Bool("noCache", options.NoCache))
	docker := spec.Build.Docker
	contextPath := filepath.Join(filepath.Dir(manifestPath), docker.Context)
	ctxPath := fmt.Sprintf("tmp/build-%s.tar", uuid.New().String())
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
	resp, err := cli.ImageBuild(
		context.TODO(),
		dockerBuildContext,
		types.ImageBuildOptions{
			NoCache:    options.NoCache,
			CacheFrom:  []string{spec.Name},
			Dockerfile: docker.Dockerfile,
			BuildArgs:  buildArgs,
			Squash:     options.Squash,
			Tags:       []string{spec.Name},
		},
	)
	if err != nil {
		return err
	}
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
	return nil
}
