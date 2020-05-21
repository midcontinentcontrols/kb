package kindest

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/term"
	"github.com/google/uuid"
	"github.com/jhoonb/archivex"
	"go.uber.org/zap"
)

type DockerBuildArg struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type EnvVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type EnvSpec struct {
	Resources []string       `json:"resources,omitempty"`
	Charts    []*ChartSpec   `json:"charts,omitempty"`
	Variables []*EnvVariable `json:"variables,omitempty"`
}

type DockerBuildSpec struct {
	Dockerfile string            `json:"dockerfile"`
	Context    string            `json:"context,omitempty"`
	BuildArgs  []*DockerBuildArg `json:"buildArgs,omitempty"`
}

type BuildSpec struct {
	Name   string           `json:"name"`
	Docker *DockerBuildSpec `json:"docker,omitempty"`
}

func (b *BuildSpec) verifyDocker(manifestPath string) error {
	var path string
	var err error
	if b.Docker.Dockerfile != "" {
		path = filepath.Join(filepath.Dir(manifestPath), b.Docker.Dockerfile)
	} else {
		path = filepath.Join(filepath.Dir(manifestPath), "Dockerfile")
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("missing Dockerfile at '%s'", path)
	}
	return nil
}

func (b *BuildSpec) Verify(manifestPath string) error {
	if b.Docker != nil {
		if b.Name == "" {
			return fmt.Errorf("missing image name")
		}
		return b.verifyDocker(manifestPath)
	} else if b.Name == "" {
		return nil
	}
	return fmt.Errorf("missing build spec")
}

func (b *BuildSpec) Build(
	manifestPath string,
	options *BuildOptions,
	cli client.APIClient,
	respHandler func(io.ReadCloser) error,
) error {
	docker := b.Docker
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
	tag := options.Tag
	if tag == "" {
		tag = "latest"
	}
	tag = fmt.Sprintf("%s:%s", b.Name, tag)
	log.Info("Building",
		zap.String("tag", tag),
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
		docker.Dockerfile,
		docker.Context,
	)
	if err != nil {
		return err
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

type ChartSpec struct {
	ReleaseName string                 `json:"releaseName"`
	Path        string                 `json:"path"`
	Values      map[string]interface{} `json:"values,omitempty"`
}

type TestSpec struct {
	Name  string    `json:"name"`
	Build BuildSpec `json:"build"`
	Env   *EnvSpec  `json:"env,omitempty"`
}

type KindestSpec struct {
	Dependencies []string    `json:"dependencies,omitempty"`
	Build        BuildSpec   `json:"build"`
	Test         []*TestSpec `json:"test,omitempty"`
}

func (s *KindestSpec) Validate(manifestPath string) error {
	if err := s.Build.Verify(manifestPath); err != nil {
		return err
	}
	rootDir := filepath.Dir(manifestPath)
	for i, dep := range s.Dependencies {
		path := filepath.Join(rootDir, dep, "kindest.yaml")
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("dependency %d: missing kindest.yaml at '%s'", i, path)
		}
	}
	for _, test := range s.Test {
		if test.Env != nil {
			for _, resource := range test.Env.Resources {
				resourcePath := filepath.Clean(filepath.Join(rootDir, resource))
				if _, err := os.Stat(resourcePath); err != nil {
					return fmt.Errorf("test '%s' env: '%s' not found", test.Name, resourcePath)
				}
			}
			for _, chart := range test.Env.Charts {
				chartPath := filepath.Join(chart.Path, "Chart.yaml")
				if _, err := os.Stat(chartPath); err != nil {
					return fmt.Errorf("test '%s' env chart '%s': missing Chart.yaml at '%s'", test.Name, chart.ReleaseName, chartPath)
				}
				valuesPath := filepath.Join(chart.Path, "values.yaml")
				if _, err := os.Stat(valuesPath); err != nil {
					return fmt.Errorf("test '%s' env chart '%s': missing values.yaml at '%s'", test.Name, chart.ReleaseName, chartPath)
				}
			}
		}
		if err := test.Build.Verify(manifestPath); err != nil {
			return err
		}
	}
	return nil
}
