package kinder

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
	Dockerfile string           `json:"dockerfile"`
	Context    string           `json:"context,omitempty"`
	BuildArgs  []DockerBuildArg `json:"buildArgs,omitempty"`
}

type BuildSpec struct {
	Docker *DockerBuildSpec `json:"docker,omitempty"`
}

func (b *BuildSpec) Verify(rootPath string) error {
	if docker := b.Docker; docker != nil {
	} else {
		return fmt.Errorf("missing build spec")
	}
	return nil
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

type KinderSpec struct {
	Name         string    `json:"name"`
	Dependencies []string  `json:"dependencies,omitempty"`
	Build        BuildSpec `json:"build"`
	Test         *TestSpec `json:"test,omitempty"`
}

func (s *KinderSpec) Validate(rootPath string) error {
	if s.Name == "" {
		return fmt.Errorf("missing name")
	}
	for i, dep := range s.Dependencies {
		path := filepath.Join(rootPath, dep, "kinder.yaml")
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("dependency %d: missing kinder.yaml at '%s'", i, path)
		}
	}
	if err := s.Build.Verify(rootPath); err != nil {
		return err
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
		if err := test.Build.Verify(rootPath); err != nil {
			return err
		}
	}
	return nil
}
