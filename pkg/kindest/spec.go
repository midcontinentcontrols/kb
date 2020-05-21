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

type EnvSpec struct {
	Resources []string       `json:"resources,omitempty"`
	Charts    []*ChartSpec   `json:"charts,omitempty"`
	Variables []*EnvVariable `json:"variables,omitempty"`
}

type ChartSpec struct {
	ReleaseName string                 `json:"releaseName"`
	Path        string                 `json:"path"`
	Values      map[string]interface{} `json:"values,omitempty"`
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
