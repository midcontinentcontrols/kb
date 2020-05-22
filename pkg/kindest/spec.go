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

type KindEnvSpec struct {
	Resources []string     `json:"resources,omitempty"`
	Charts    []*ChartSpec `json:"charts,omitempty"`
}

type DockerEnvSpec struct {
}

type EnvSpec struct {
	Kind      *KindEnvSpec   `json:"kind,omitempty"`
	Docker    *DockerEnvSpec `json:"docker,omitempty"`
	Variables []*EnvVariable `json:"variables,omitempty"`
}

type ChartSpec struct {
	ReleaseName string                 `json:"releaseName"`
	Name        string                 `json:"Name"`
	Values      map[string]interface{} `json:"values,omitempty"`
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
