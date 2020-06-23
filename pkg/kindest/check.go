package kindest

import (
	"fmt"
	"sync"

	"github.com/midcontinentcontrols/kindest/pkg/logger"
)

type BuildStatus string

const (
	BuildStatusPending   BuildStatus = "Pending"
	BuildStatusFailed    BuildStatus = "Failed"
	BuildStatusSucceeded BuildStatus = "Succeeded"
)

type resolver struct {
	l       sync.Mutex         //
	modules map[string]*Module // map of manifestPath to *Module
}

type Module struct {
	ManifestPath string    // absolute path to manifest
	Dependencies []*Module //
	Status       BuildStatus
	l            sync.Mutex
}

func NewModule(
	manifestPath string,
	dependencies []*Module,
) *Module {
	return &Module{
		ManifestPath: manifestPath,
		Dependencies: dependencies,
		Status:       BuildStatusPending,
	}
}

func (m *Module) Build() (err error) {
	m.l.Lock()
	defer func() {
		if err == nil {
			m.Status = BuildStatusSucceeded
		} else {
			m.Status = BuildStatusFailed
		}
		m.l.Unlock()
	}()
	switch m.Status {
	case BuildStatusPending:
		break
	case BuildStatusFailed:
		return fmt.Errorf("build failed")
	case BuildStatusSucceeded:
		return nil
	default:
		panic("unreachable")
	}
	return nil
}

type CheckOptions struct {
}

func Check(options *CheckOptions, log logger.Logger) (*Module, error) {
	return nil, nil
}
