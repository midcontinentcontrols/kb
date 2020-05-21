package kindest

import (
	"fmt"
	"os"
	"path/filepath"
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
