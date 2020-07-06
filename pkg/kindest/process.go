package kindest

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/midcontinentcontrols/kindest/pkg/logger"
)

type Process struct {
	l       sync.Mutex
	modules map[string]*Module
	log     logger.Logger
}

func NewProcess(log logger.Logger) *Process {
	return &Process{
		modules: make(map[string]*Module),
		log:     log,
	}
}

func (p *Process) GetModule(dir string) (*Module, error) {
	dir, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		return nil, err
	}
	p.l.Lock()
	defer p.l.Unlock()
	if existing, ok := p.modules[dir]; ok {
		return existing, nil
	}
	manifestPath := filepath.Join(dir, "kindest.yaml")
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
	m := &Module{
		Spec:         spec,
		ManifestPath: manifestPath,
		Dependencies: dependencies,
		log:          p.log,
	}
	p.modules[dir] = m
	return m, nil
}
