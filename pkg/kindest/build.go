package kindest

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"runtime"

	"github.com/Jeffail/tunny"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/term"
	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"github.com/jhoonb/archivex"
	"github.com/monochromegane/go-gitignore"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
)

type DockerBuildArg struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type BuildSpec struct {
	Name       string            `json:"name"`
	Dockerfile string            `json:"dockerfile,omitempty"`
	Context    string            `json:"context,omitempty"`
	BuildArgs  []*DockerBuildArg `json:"buildArgs,omitempty"`
}

func (b *BuildSpec) verifyDocker(manifestPath string) error {
	var path string
	if b.Dockerfile != "" {
		path = filepath.Join(filepath.Dir(manifestPath), b.Dockerfile)
	} else {
		path = filepath.Join(filepath.Dir(manifestPath), "Dockerfile")
	}
	path = filepath.Clean(path)
	log.Info("Resolving Dockerfile", zap.String("name", b.Name), zap.String("path", path))
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("missing Dockerfile at '%s'", path)
	}
	return nil
}

var ErrMissingImageName = fmt.Errorf("missing image name")

func (b *BuildSpec) Verify(manifestPath string) error {
	if b.Name == "" {
		return ErrMissingImageName
	}
	return b.verifyDocker(manifestPath)
}

func (b *BuildSpec) buildDocker(
	manifestPath string,
	options *BuildOptions,
	cli client.APIClient,
	respHandler func(io.ReadCloser) error,
) error {
	contextPath := filepath.Clean(filepath.Join(filepath.Dir(manifestPath), b.Context))
	u, err := user.Current()
	if err != nil {
		return err
	}
	tmpDir := filepath.Join(u.HomeDir, ".kindest", "tmp")
	if err := os.MkdirAll(tmpDir, 0766); err != nil {
		return err
	}
	ctxPath := filepath.Join(tmpDir, fmt.Sprintf("build-context-%s.tar", uuid.New().String()))
	tag := options.Tag
	if tag == "" {
		tag = "latest"
	}
	resolvedDockerfile, err := resolveDockerfile(
		manifestPath,
		b.Dockerfile,
		b.Context,
	)
	if err != nil {
		return err
	}
	tag = fmt.Sprintf("%s:%s", b.Name, tag)
	log.Info("Building",
		zap.String("resolvedDockerfile", resolvedDockerfile),
		zap.String("context", contextPath),
		zap.String("tag", tag),
		zap.Bool("noCache", options.NoCache))
	tar := new(archivex.TarFile)
	tar.Create(ctxPath)
	dockerignorePath := filepath.Join(contextPath, ".dockerignore")
	if _, err := os.Stat(dockerignorePath); err == nil {
		dockerignore, err := gitignore.NewGitIgnore(dockerignorePath, contextPath)
		if err != nil {
			return err
		}
		if err := filepath.Walk(contextPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if dockerignore.Match(path, info.IsDir()) {
				if !info.IsDir() {
					abs, err := filepath.Abs(path)
					if err != nil {
						return err
					}
					f, err := os.Open(abs)
					if err != nil {
						return err
					}
					defer f.Close()
					rel, err := filepath.Rel(contextPath, abs)
					if err != nil {
						return err
					}
					//log.Info("Adding file to build context", zap.String("rel", rel), zap.String("abs", abs))
					if err := tar.Add(rel, f, info); err != nil {
						return err
					}
				}
			}
			return nil
		}); err != nil {
			return err
		}
	} else if err := tar.AddAll(contextPath, false); err != nil {
		return err
	}
	if err := tar.Close(); err != nil {
		return err
	}
	defer os.Remove(ctxPath)
	dockerBuildContext, err := os.Open(ctxPath)
	if err != nil {
		return err
	}
	defer dockerBuildContext.Close()
	buildArgs := make(map[string]*string)
	for _, arg := range b.BuildArgs {
		buildArgs[arg.Name] = &arg.Value
	}
	resp, err := cli.ImageBuild(
		context.TODO(),
		dockerBuildContext,
		types.ImageBuildOptions{
			NoCache:    options.NoCache,
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
		if err := respHandler(resp.Body); err != nil {
			return err
		}
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
	if options.Push {
		log := log.With(zap.String("tag", tag))
		log.Info("Pushing image")
		authConfig, err := RegistryAuthFromEnv()
		if err != nil {
			return err
		}
		log.Info("Using docker credentials from env", zap.String("username", string(authConfig.Username)))
		authBytes, err := json.Marshal(authConfig)
		if err != nil {
			return err
		}
		registryAuth := base64.URLEncoding.EncodeToString(authBytes)
		resp, err := cli.ImagePush(
			context.TODO(),
			tag,
			types.ImagePushOptions{
				All:          false,
				RegistryAuth: registryAuth,
			},
		)
		if err != nil {
			return err
		}
		termFd, isTerm := term.GetFdInfo(os.Stderr)
		if err := jsonmessage.DisplayJSONMessagesStream(
			resp,
			os.Stderr,
			termFd,
			isTerm,
			nil,
		); err != nil {
			return err
		}
		log.Info("Pushed image")
	}
	return nil
}
func (b *BuildSpec) Build(
	manifestPath string,
	options *BuildOptions,
	cli client.APIClient,
	respHandler func(io.ReadCloser) error,
) error {
	return b.buildDocker(
		manifestPath,
		options,
		cli,
		respHandler,
	)
}

type BuildOptions struct {
	File        string `json:"file,omitempty"`
	NoCache     bool   `json:"nocache,omitempty"`
	Squash      bool   `json:"squash,omitempty"`
	Tag         string `json:"tag,omitempty"`
	Concurrency int    `json:"concurrency,omitempty"`
	Push        bool   `json:"push,omitempty"`
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

func locateSpec(file string) (string, error) {
	if file != "" {
		var err error
		file, err = filepath.Abs(file)
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

func resolveDockerfile(manifestPath string, dockerfilePath string, contextPath string) (string, error) {
	if dockerfilePath == "" {
		dockerfilePath = "Dockerfile"
	}
	rootDir := filepath.Dir(manifestPath)
	dockerfilePath = filepath.Clean(filepath.Join(rootDir, dockerfilePath))
	contextPath = filepath.Clean(filepath.Join(rootDir, contextPath))
	rel, err := filepath.Rel(contextPath, dockerfilePath)
	if err != nil {
		return "", err
	}
	return rel, nil
}

func loadSpec(file string) (*KindestSpec, string, error) {
	manifestPath, err := locateSpec(file)
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
	concurrency := options.Concurrency
	if concurrency == 0 {
		concurrency = runtime.NumCPU()
	}
	pool = tunny.NewFunc(concurrency, func(payload interface{}) interface{} {
		options := payload.(*BuildOptions)
		return BuildEx(options, cli, pool, nil)
	})
	defer pool.Close()
	return BuildEx(options, cli, pool, nil)
}

func RegistryAuthFromEnv() (*types.AuthConfig, error) {
	username, ok := os.LookupEnv("DOCKER_USERNAME")
	if !ok {
		return nil, fmt.Errorf("missing DOCKER_USERNAME")
	}
	password, ok := os.LookupEnv("DOCKER_PASSWORD")
	if !ok {
		return nil, fmt.Errorf("missing DOCKER_PASSWORD")
	}
	return &types.AuthConfig{
		Username: string(username),
		Password: string(password),
	}, nil
}

func BuildEx(
	options *BuildOptions,
	cli client.APIClient,
	pool *tunny.Pool,
	respHandler func(io.ReadCloser) error,
) error {
	spec, manifestPath, err := loadSpec(options.File)
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
	if spec.Build != nil {
		if err := spec.Build.Build(
			manifestPath,
			options,
			cli,
			respHandler,
		); err != nil {
			return err
		}
	}
	return nil
}

/*
func detectErrorMessage(in io.Reader) error {
	dec := json.NewDecoder(in)
	for {
		var jm jsonmessage.JSONMessage
		if err := dec.Decode(&jm); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		log.Info("Push", zap.String("message", fmt.Sprintf("%+v", jm)))
		// skip progress message
		//if jm.Progress == nil {
		//glog.Infof("%v", jm)
		//}
		if jm.Error != nil {
			return jm.Error
		}

		if len(jm.ErrorMessage) > 0 {
			return errors.New(jm.ErrorMessage)
		}
	}
	return nil
}
*/
