package kindest

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/Jeffail/tunny"
	"github.com/midcontinentcontrols/kindest/pkg/logger"
)

type buildJob struct {
	m       *Module
	options *BuildOptions
}

type Process struct {
	pool    *tunny.Pool
	l       sync.Mutex
	modules map[string]*Module
	log     logger.Logger
}

func NewProcess(concurrency int, log logger.Logger) *Process {
	pool := tunny.NewFunc(concurrency, func(payload interface{}) interface{} {
		if build, ok := payload.(*buildJob); ok {
			return build.m.doBuild(
				build.options,
			)
		}
		panic("unreachable branch detected")
	})
	return &Process{
		modules: make(map[string]*Module),
		log:     log,
		pool:    pool,
	}
}

func (p *Process) Close() {
	p.pool.Close()
}

func (p *Process) GetModule(manifestPath string) (*Module, error) {
	p.l.Lock()
	defer p.l.Unlock()
	return p.getModuleNoLock(manifestPath)
}

func (p *Process) GetModuleFromBuildSpec(manifestPath string, b *BuildSpec) (*Module, error) {
	return &Module{
		Path: manifestPath,
		log:  p.log,
		pool: p.pool,
		Spec: &KindestSpec{
			Build: b,
		},
	}, nil
}

func (p *Process) getModuleNoLock(manifestPath string) (*Module, error) {
	manifestPath, err := filepath.Abs(filepath.Clean(manifestPath))
	if err != nil {
		return nil, err
	}
	if existing, ok := p.modules[manifestPath]; ok {
		return existing, nil
	}
	spec, _, err := loadSpec(manifestPath, p.log)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(manifestPath)
	var dependencies []*Module
	for _, dependency := range spec.Dependencies {
		dep, err := p.getModuleNoLock(filepath.Clean(filepath.Join(dir, dependency, "kindest.yaml")))
		if err != nil {
			return nil, fmt.Errorf("dependency '%s': %v", dependency, err)
		}
		dependencies = append(dependencies, dep)
	}
	m := &Module{
		Spec:         spec,
		Path:         manifestPath,
		Dependencies: dependencies,
		log:          p.log,
		pool:         p.pool,
	}
	p.modules[manifestPath] = m
	return m, nil
}
