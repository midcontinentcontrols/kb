package kindest

import (
	"fmt"

	"github.com/midcontinentcontrols/kindest/pkg/logger"

	"os"
	"path/filepath"
)

type EnvVariable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type KubernetesEnvSpec struct {
	Resources []string     `json:"resources,omitempty" yaml:"resources,omitempty"`
	Charts    []*ChartSpec `json:"charts,omitempty" yaml:"charts,omitempty"`
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

type DockerEnvSpec struct {
}

func (d *DockerEnvSpec) Verify(manifestPath string) error {
	return nil
}

type EnvSpec struct {
	Kubernetes *KubernetesEnvSpec `json:"kubernetes,omitempty" yaml:"kubernetes,omitempty"`
	Docker     *DockerEnvSpec     `json:"docker,omitempty" yaml:"docker,omitempty"`
	Variables  []*EnvVariable     `json:"variables,omitempty" yaml:"variables,omitempty"`
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

type KindestSpec struct {
	Dependencies []string    `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	Build        *BuildSpec  `json:"build" yaml:"build"`
	Test         []*TestSpec `json:"test,omitempty" yaml:"test,omitempty"`
}

func (s *KindestSpec) Verify(manifestPath string, log logger.Logger) error {
	if s.Build != nil {
		if err := s.Build.Verify(manifestPath, log); err != nil {
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
		if err := test.Verify(manifestPath, log); err != nil {
			return err
		}
	}
	return nil
}
