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

type EnvSpec struct {
	Name  string `json:"name"`
	Value string `json:"value"`
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
	Charts []*ChartSpec `json:"charts"`
	Build  BuildSpec    `json:"build"`
	Env    []*EnvSpec   `json:"env,omitempty"`
}

type KindestSpec struct {
	Dependencies []string  `json:"dependencies,omitempty"`
	Build        BuildSpec `json:"build"`
	Test         *TestSpec `json:"test,omitempty"`
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
	if test := s.Test; test != nil {
		for i, chart := range test.Charts {
			chartPath := filepath.Join(chart.Path, "Chart.yaml")
			if _, err := os.Stat(chartPath); err != nil {
				return fmt.Errorf("test env chart %d: missing Chart.yaml at '%s'", i, chartPath)
			}

			valuesPath := filepath.Join(chart.Path, "values.yaml")
			if _, err := os.Stat(valuesPath); err != nil {
				return fmt.Errorf("test env chart %d: missing values.yaml at '%s'", i, chartPath)
			}
		}
		if err := test.Build.Verify(manifestPath); err != nil {
			return err
		}
	}
	return nil
}
