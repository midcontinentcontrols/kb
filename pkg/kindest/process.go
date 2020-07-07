package kindest

import (
	"fmt"
	"path/filepath"
	"runtime"
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

func NewProcess(log logger.Logger) *Process {
	pool := tunny.NewFunc(runtime.NumCPU(), func(payload interface{}) interface{} {
		if build, ok := payload.(*buildJob); ok {
			return build.m.Build(build.options)
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

func (p *Process) GetModule(dir string) (*Module, error) {
	p.l.Lock()
	defer p.l.Unlock()
	return p.getModuleNoLock(dir)
}

func (p *Process) getModuleNoLock(dir string) (*Module, error) {
	dir, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		return nil, err
	}
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
		dep, err := p.getModuleNoLock(filepath.Clean(filepath.Join(dir, dependency)))
		if err != nil {
			return nil, fmt.Errorf("dependency '%s': %v", dependency, err)
		}
		dependencies = append(dependencies, dep)
	}
	m := &Module{
		Spec:         spec,
		Dir:          dir,
		Dependencies: dependencies,
		log:          p.log,
		pool:         p.pool,
	}
	p.modules[dir] = m
	return m, nil
}
