package kindest

import (
	"fmt"
	"os"
	"path/filepath"
)

type EnvVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type KubernetesEnvSpec struct {
	Resources []string     `json:"resources,omitempty"`
	Charts    []*ChartSpec `json:"charts,omitempty"`
}

func (k *KubernetesEnvSpec) Verify(manifestPath string) error {
	rootDir := filepath.Dir(manifestPath)
	for _, resource := range k.Resources {
		resourcePath := filepath.Clean(filepath.Join(rootDir, resource))
		if _, err := os.Stat(resourcePath); err != nil {
			return fmt.Errorf("resource '%s' not found", resourcePath)
		}
	}
	for _, chart := range k.Charts {
		chartPath := filepath.Join(chart.Name, "Chart.yaml")
		if _, err := os.Stat(chartPath); err != nil {
			return fmt.Errorf("test '%s' env chart '%s': missing Chart.yaml at '%s'", t.Name, chart.ReleaseName, chartPath)
		}
		valuesPath := filepath.Join(chart.Name, "values.yaml")
		if _, err := os.Stat(valuesPath); err != nil {
			return fmt.Errorf("test '%s' env chart '%s': missing values.yaml at '%s'", t.Name, chart.ReleaseName, chartPath)
		}
	}
	return nil
}

type DockerEnvSpec struct {
}

func (d *DockerEnvSpec) Verify(manifestPath string) error {
	return nil
}

type EnvSpec struct {
	Kubernetes *KubernetesEnvSpec `json:"kubernetes,omitempty"`
	Docker     *DockerEnvSpec     `json:"docker,omitempty"`
	Variables  []*EnvVariable     `json:"variables,omitempty"`
}

type ChartSpec struct {
	ReleaseName    string                 `json:"releaseName"`
	Namespace      string                 `json:"namespace,omitempty"`
	Path           string                 `json:"path,omitempty"`
	RepoURL        string                 `json:"repoURL,omitempty"`
	TargetRevision string                 `json:"targetRevision,omitempty"`
	Values         map[string]interface{} `json:"values,omitempty"`
	ValuesFiles    []string               `json:"valuesFiles,omitempty"`
}

type KindestSpec struct {
	Dependencies []string    `json:"dependencies,omitempty"`
	Build        *BuildSpec  `json:"build"`
	Test         []*TestSpec `json:"test,omitempty"`
}

func (s *KindestSpec) Validate(manifestPath string) error {
	if s.Build != nil {
		if err := s.Build.Verify(manifestPath); err != nil {
			return err
		}
	}
	rootDir := filepath.Dir(manifestPath)
	for i, dep := range s.Dependencies {
		path := filepath.Join(rootDir, dep, "kindest.yaml")
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("dependency %d: missing kindest.yaml at '%s'", i, path)
		}
	}
	for _, test := range s.Test {
		if err := test.Verify(manifestPath); err != nil {
			return err
		}
	}
	return nil
}
