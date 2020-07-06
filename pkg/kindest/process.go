package kindest

import (
	"fmt"
	"path/filepath"

	"github.com/midcontinentcontrols/kindest/pkg/logger"
)

type Process struct {
	modules map[string]*Module
	log     logger.Logger
}

func NewProcess(log logger.Logger) *Process {
	return &Process{
		modules: make(map[string]*Module),
		log:     log,
	}
}

func (p *Process) GetModule(manifestPath string) (*Module, error) {
	spec, _, err := loadSpec(manifestPath, p.log)
	if err != nil {
		return nil, err
	}
	var dependencies []*Module
	for _, dependency := range spec.Dependencies {
		dep, err := p.GetModule(filepath.Clean(filepath.Join(dir, dependency)))
		if err != nil {
			return nil, fmt.Errorf("dependency '%s': %v", dependency, err)
		}
		dependencies = append(dependencies, dep)
	}
	return &Module{
		Spec:         spec,
		ManifestPath: manifestPath,
		Dependencies: dependencies,
		log:          p.log,
	}, nil
}
