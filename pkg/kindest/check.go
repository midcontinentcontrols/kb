package kindest

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"hash"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"unsafe"

	"go.uber.org/zap"

	"github.com/midcontinentcontrols/kindest/pkg/logger"
)

type BuildStatus int32

func (b BuildStatus) String() string {
	switch b {
	case BuildStatusPending:
		return "Pending"
	case BuildStatusInProgress:
		return "InProgress"
	case BuildStatusFailed:
		return "Failed"
	case BuildStatusSucceeded:
		return "Succeeded"
	default:
		return fmt.Sprintf("Unknown (%d)", int32(b))
	}
}

const (
	BuildStatusPending    BuildStatus = 0
	BuildStatusInProgress BuildStatus = 1
	BuildStatusFailed     BuildStatus = 2
	BuildStatusSucceeded  BuildStatus = 3
)

type resolver struct {
	l       sync.Mutex         //
	modules map[string]*Module // map of manifestPath to *Module
}

type Module struct {
	Spec         *KindestSpec
	Relative     string
	ManifestPath string    // absolute path to manifest
	Dependencies []*Module //
	status       int32
	subscribersL sync.Mutex
	subscribers  []chan<- error
	err          unsafe.Pointer
	log          logger.Logger
}

// NewModule constructor for the Module type.
// `spec` is expected to be validated.
func NewModule(
	spec *KindestSpec,
	relative string,
	manifestPath string,
	dependencies []*Module,
) *Module {
	return &Module{
		Spec:         spec,
		Relative:     relative,
		ManifestPath: manifestPath,
		Dependencies: dependencies,
	}
}

var ErrModuleNotCached = fmt.Errorf("module is not cached")

func (m *Module) CachedDigest() (string, error) {
	path, err := digestPathForManifest(m.ManifestPath)
	if err != nil {
		return "", err
	}
	body, err := ioutil.ReadFile(path)
	if err != nil {
		return "", errDigestNotCached
	}
	return string(body), ErrModuleNotCached
}

func (m *Module) cacheDigest(digest string) error {
	path, err := digestPathForManifest(m.ManifestPath)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err = os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	if err := ioutil.WriteFile(path, []byte(digest), 0644); err != nil {
		return err
	}
	return nil
}

func (m *Module) buildDependencies() error {
	for _, dependency := range m.Dependencies {
		if err := dependency.Build(); err != nil {
			return fmt.Errorf("%s: %v", m.Relative, err)
		}
	}
	return nil
}

func (m *Module) loadBuildContext() (BuildContext, error) {
	c := make(map[string]interface{})
	// TODO: copy over code with dockerignore, maybe do some refactoring
	return BuildContext(c), nil
}

func (m *Module) Status() BuildStatus {
	return BuildStatus(atomic.LoadInt32(&m.status))
}

func (m *Module) setStatus(status BuildStatus) error {
	atomic.StoreInt32(&m.status, int32(status))
	return nil
}

func (m *Module) claim() bool {
	return atomic.CompareAndSwapInt32(
		&m.status,
		int32(BuildStatusPending),
		int32(BuildStatusInProgress),
	)
}

func (m *Module) subscribe(done chan<- error) {
	m.subscribersL.Lock()
	defer m.subscribersL.Unlock()
	switch m.Status() {
	case BuildStatusInProgress:
		m.subscribers = append(m.subscribers, done)
	case BuildStatusFailed:
		// m.err may be nil because of threading volatility
		box := (*string)(atomic.LoadPointer(&m.err))
		done <- fmt.Errorf(*box)
	case BuildStatusSucceeded:
		done <- nil
	default:
		panic("unreachable")
	}
}

func (m *Module) broadcast(err error) {
	m.subscribersL.Lock()
	defer m.subscribersL.Unlock()
	msg := err.Error()
	atomic.StorePointer(&m.err, unsafe.Pointer(&msg))
	if err == nil {
		err = m.setStatus(BuildStatusSucceeded)
	} else {
		m.setStatus(BuildStatusFailed)
	}
	for _, subscriber := range m.subscribers {
		subscriber <- err
		close(subscriber)
	}
}

func (m *Module) WaitForCompletion() error {
	done := make(chan error, 1)
	m.subscribe(done)
	return <-done
}

func (m *Module) Build() (err error) {
	if !m.claim() {
		switch m.Status() {
		case BuildStatusInProgress:
			return m.WaitForCompletion()
		case BuildStatusFailed:
			box := (*string)(atomic.LoadPointer(&m.err))
			if box == nil {
				panic("unreachable")
			}
			return fmt.Errorf(*box)
		case BuildStatusSucceeded:
			return nil
		default:
			panic("unreachable")
		}
	}
	defer func() {
		m.broadcast(err)
	}()
	if err := m.buildDependencies(); err != nil {
		return err
	}
	buildContext, err := m.loadBuildContext()
	if err != nil {
		return err
	}
	digest, err := buildContext.Digest()
	if err != nil {
		return err
	}
	cachedDigest, err := m.CachedDigest()
	if err != nil && err != ErrModuleNotCached {
		return err
	}
	if digest == cachedDigest {
		m.log.Info("No files changed", zap.String("digest", cachedDigest))
		return nil
	}
	// TODO: actually do the building
	if err := m.cacheDigest(digest); err != nil {
		return err
	}
	return nil
}

type BuildContext map[string]interface{}

func hashContext(context map[string]interface{}, h hash.Hash, prefix string) error {
	for k, v := range context {
		k = prefix + k
		if file, ok := v.(*File); ok {
			if _, err := h.Write([]byte(k)); err != nil {
				return err
			}
			if _, err := h.Write([]byte{
				byte((file.Permissions >> 24) & 0xFF),
				byte((file.Permissions >> 16) & 0xFF),
				byte((file.Permissions >> 8) & 0xFF),
				byte((file.Permissions >> 0) & 0xFF),
			}); err != nil {
				return err
			}
			if _, err := h.Write(file.Content); err != nil {
				return err
			}
		} else if dir, ok := v.(*Directory); ok {
			k = k + "?" // impossible to use in most filesystems
			if _, err := h.Write([]byte(k)); err != nil {
				return err
			}
			if _, err := h.Write([]byte{
				byte((dir.Permissions >> 24) & 0xFF),
				byte((dir.Permissions >> 16) & 0xFF),
				byte((dir.Permissions >> 8) & 0xFF),
				byte((dir.Permissions >> 0) & 0xFF),
			}); err != nil {
				return err
			}
			if err := hashContext(dir.Contents, h, k); err != nil {
				return err
			}
		} else {
			panic("unreachable")
		}
	}
	return nil
}

func (c BuildContext) Digest() (string, error) {
	h := md5.New()
	if err := hashContext(map[string]interface{}(c), h, ""); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (c BuildContext) Archive() ([]byte, error) {
	return nil, nil
}

type File struct {
	Permissions int
	Content     []byte
}

type Directory struct {
	Permissions int
	Contents    map[string]interface{}
}

type CheckOptions struct {
}

func Check(options *CheckOptions, log logger.Logger) (*Module, error) {
	return nil, nil
}
