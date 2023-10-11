package kb

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/midcontinentcontrols/kb/pkg/logger"

	"os"
	"path/filepath"
)

type Variable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type KubernetesEnvSpec struct {
	ImagePullSecret string                `json:"imagePullSecret,omitempty"`
	Resources       []string              `json:"resources,omitempty" yaml:"resources,omitempty"`
	Charts          map[string]*ChartSpec `json:"charts,omitempty" yaml:"charts,omitempty"`
}

var ErrMultipleChartSources = fmt.Errorf("multiple chart sources not allowed")

var ErrMissingChartSource = fmt.Errorf("missing chart source")

var ErrMissingReleaseName = fmt.Errorf("missing release name")

func (k *KubernetesEnvSpec) verifyResources(rootDir string) error {
	for _, resource := range k.Resources {
		resourcePath := filepath.Clean(filepath.Join(rootDir, resource))
		if _, err := os.Stat(resourcePath); err != nil {
			return fmt.Errorf("resource '%s' not found", resourcePath)
		}
	}
	return nil
}

func (k *KubernetesEnvSpec) verifyCharts(rootDir string, log logger.Logger) error {
	for _, chart := range k.Charts {
		if chart.Name == "" {
			return ErrMissingChartSource
		}
		if chart.ReleaseName == "" {
			log.Info(fmt.Sprintf("%#v", chart))
			return ErrMissingReleaseName
		}
		chartDir := filepath.Clean(filepath.Join(rootDir, chart.Name))
		chartPath := filepath.Join(chartDir, "Chart.yaml")
		if _, err := os.Stat(chartPath); err != nil {
			return fmt.Errorf("missing Chart.yaml at %s", chartPath)
		}
		valuesPath := filepath.Join(chartDir, "values.yaml")
		if _, err := os.Stat(valuesPath); err != nil {
			return fmt.Errorf("missing values.yaml at %s", valuesPath)
		}
	}
	return nil
}

func (k *KubernetesEnvSpec) Verify(manifestPath string, log logger.Logger) error {
	rootDir := filepath.Dir(manifestPath)
	if err := k.verifyResources(rootDir); err != nil {
		return err
	}
	if err := k.verifyCharts(rootDir, log); err != nil {
		return err
	}
	return nil
}

type DockerVolumeSpec struct {
	Type        string `json:"type,omitempty"`
	Source      string `json:"source,omitempty"`
	Target      string `json:"target,omitempty"`
	ReadOnly    bool   `json:"readOnly,omitempty"`
	Consistency string `json:"consistency,omitempty"`
}

type DockerEnvSpec struct {
	Volumes []*DockerVolumeSpec `json:"volumes,omitempty"`
}

func (d *DockerEnvSpec) Verify(manifestPath string) error {
	return nil
}

type EnvSpec struct {
	Kubernetes *KubernetesEnvSpec `json:"kubernetes,omitempty" yaml:"kubernetes,omitempty"`
	Docker     *DockerEnvSpec     `json:"docker,omitempty" yaml:"docker,omitempty"`
}

type ChartSpec struct {
	ReleaseName    string                      `json:"releaseName" yaml:"releaseName"`
	Namespace      string                      `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Name           string                      `json:"name,omitempty" yaml:"name,omitempty"`
	RepoURL        string                      `json:"repoURL,omitempty" yaml:"repoURL,omitempty"`
	TargetRevision string                      `json:"targetRevision,omitempty" yaml:"targetRevision,omitempty"`
	Values         map[interface{}]interface{} `json:"values,omitempty" yaml:"values,omitempty"`
	ValuesFiles    []string                    `json:"valuesFiles,omitempty" yaml:"valuesFiles,omitempty"`
}

type ModuleSpec struct {
	Dependencies []string    `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	Build        *BuildSpec  `json:"build" yaml:"build"`
	Env          EnvSpec     `json:"env,omitempty" yaml:"env,omitempty"`
	Test         []*TestSpec `json:"test,omitempty" yaml:"test,omitempty"`
}

type TestSpec struct {
	Name           string      `json:"name"`
	Build          *BuildSpec  `json:"build"`
	Variables      []*Variable `json:"variables,omitempty" yaml:"variables,omitempty"`
	Env            EnvSpec     `json:"env,omitempty" yaml:"env,omitempty"`
	DefaultTimeout string      `json:"defaultTimeout,omitempty" yaml:"defaultTimeout,omitempty"`
}

func (b *BuildSpec) GetDockerfilePath(rootPath string) string {
	dockerfilePath := b.Dockerfile
	if dockerfilePath == "" {
		dockerfilePath = "Dockerfile"
	}
	dockerfilePath = filepath.Clean(filepath.Join(rootPath, dockerfilePath))
	return dockerfilePath
}

func (b *BuildSpec) ReadDockerfile(rootPath string) (string, error) {
	body, err := ioutil.ReadFile(b.GetDockerfilePath(rootPath))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func tokenize(x string) []string {
	x = strings.ReplaceAll(x, "\t", " ")
	x = strings.ReplaceAll(x, "\r\n", " ")
	x = strings.ReplaceAll(x, "\n", " ")
	parts := strings.Split(x, " ")
	var result []string
	for _, part := range parts {
		if len(part) > 0 {
			result = append(result, part)
		}
	}
	return result
}

func (b *BuildSpec) GetBaseImage(rootPath string) (string, error) {
	dockerfilePath := b.GetDockerfilePath(rootPath)
	f, err := os.Open(dockerfilePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	lineno := 0
	variables := make(map[string]string)
	for scanner.Scan() {
		lineno++
		line := strings.TrimSpace(scanner.Text())
		tokens := tokenize(line)
		numTokens := len(tokens)
		if numTokens == 0 {
			continue
		}
		switch tokens[0] {
		case "ARG":
			key := tokens[1]
			var value string
			if len(tokens) == 4 {
				// default value
				value = tokens[3]
			}
			for _, buildArg := range b.BuildArgs {
				if buildArg.Name == key {
					value = buildArg.Value
				}
			}
			variables[key] = value
		case "FROM":
			// This is assumed to be the base image
			// It could contain a build arg, in which
			// case we will have to interpolate.
			baseImage := strings.TrimSpace(tokens[1])
			for k, v := range variables {
				baseImage = strings.ReplaceAll(
					baseImage,
					fmt.Sprintf("${%s}", k),
					v,
				)
			}
			return baseImage, nil
		}
	}
	return "", fmt.Errorf("cannot find appropriate FROM directive in %s", dockerfilePath)
}

func (s *ModuleSpec) RunTests(
	options *TestOptions,
	manifestPath string,
	p *Process,
	log logger.Logger,
) error {
	// TODO: execute in parallel
	for _, test := range s.Test {
		if err := test.Run(
			options,
			manifestPath,
			p,
			log,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *ModuleSpec) Verify(manifestPath string, log logger.Logger) error {
	if s.Build != nil {
		if err := s.Build.Verify(manifestPath, log); err != nil {
			return err
		}
	}
	rootDir := filepath.Dir(manifestPath)
	for i, dep := range s.Dependencies {
		path := filepath.Join(rootDir, dep, "kb.yaml")
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("dependency %d: missing kb.yaml at '%s'", i, path)
		}
	}
	for _, test := range s.Test {
		if err := test.Verify(manifestPath, log); err != nil {
			return err
		}
	}
	return nil
}
