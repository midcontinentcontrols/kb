package kindest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/midcontinentcontrols/kindest/pkg/logger"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestModuleBuildStatus(t *testing.T) {
	t.Run("BuildStatusSucceeded", func(t *testing.T) {
		name := "test-" + uuid.New().String()[:8]
		rootPath := filepath.Join("tmp", name)
		require.NoError(t, os.MkdirAll(rootPath, 0644))
		defer func() {
			require.NoError(t, os.RemoveAll(rootPath))
		}()
		specYaml := fmt.Sprintf(`build:
  name: %s`, name)
		dockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "echo \"Hello, world\""]`
		require.NoError(t, createFiles(map[string]interface{}{
			"kindest.yaml": specYaml,
			"Dockerfile":   dockerfile,
		}, rootPath))
		p := NewProcess(logger.NewFakeLogger())
		module, err := p.GetModule(rootPath)
		require.NoError(t, err)
		require.Equal(t, BuildStatusPending, module.Status())
		require.NoError(t, module.Build(&BuildOptions{}))
		require.Equal(t, BuildStatusSucceeded, module.Status())
	})
	t.Run("BuildStatusFailed", func(t *testing.T) {
		name := "test-" + uuid.New().String()[:8]
		rootPath := filepath.Join("tmp", name)
		require.NoError(t, os.MkdirAll(rootPath, 0644))
		defer func() {
			require.NoError(t, os.RemoveAll(rootPath))
		}()
		specYaml := fmt.Sprintf(`build:
  name: %s`, name)
		dockerfile := `FROM alpine:3.11.6
RUN cat foo`
		require.NoError(t, createFiles(map[string]interface{}{
			"kindest.yaml": specYaml,
			"Dockerfile":   dockerfile,
		}, rootPath))
		p := NewProcess(logger.NewFakeLogger())
		module, err := p.GetModule(rootPath)
		require.NoError(t, err)
		err = module.Build(&BuildOptions{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "docker: The command '/bin/sh -c cat foo' returned a non-zero code: 1")
		require.Equal(t, BuildStatusFailed, module.Status())
	})
}

func TestModuleBuildContext(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		name := "test-" + uuid.New().String()[:8]
		rootPath := filepath.Join("tmp", name)
		require.NoError(t, os.MkdirAll(rootPath, 0644))
		defer func() {
			require.NoError(t, os.RemoveAll(rootPath))
		}()
		specYaml := fmt.Sprintf(`build:
  name: %s`, name)
		dockerfile := `FROM alpine:3.11.6
COPY foo foo
RUN cat foo
CMD ["sh", "-c", "echo \"Hello, world\""]`
		require.NoError(t, createFiles(map[string]interface{}{
			"kindest.yaml": specYaml,
			"foo":          "hello, world!",
			"Dockerfile":   dockerfile,
		}, rootPath))
		p := NewProcess(logger.NewFakeLogger())
		module, err := p.GetModule(rootPath)
		require.NoError(t, err)
		require.NoError(t, module.Build(&BuildOptions{}))
		require.Equal(t, BuildStatusSucceeded, module.Status())
	})
	t.Run("subdir", func(t *testing.T) {
		name := "test-" + uuid.New().String()[:8]
		rootPath := filepath.Join("tmp", name)
		require.NoError(t, os.MkdirAll(rootPath, 0644))
		defer func() {
			require.NoError(t, os.RemoveAll(rootPath))
		}()
		specYaml := fmt.Sprintf(`build:
  name: %s`, name)
		dockerfile := `FROM alpine:3.11.6
COPY subdir/foo subdir/foo
RUN cat subdir/foo
CMD ["sh", "-c", "echo \"Hello, world\""]`
		require.NoError(t, createFiles(map[string]interface{}{
			"kindest.yaml": specYaml,
			"subdir": map[string]interface{}{
				"foo": "hello, world!",
			},
			"Dockerfile": dockerfile,
		}, rootPath))
		p := NewProcess(logger.NewFakeLogger())
		module, err := p.GetModule(rootPath)
		require.NoError(t, err)
		require.NoError(t, module.Build(&BuildOptions{}))
		require.Equal(t, BuildStatusSucceeded, module.Status())
	})
	t.Run("dockerignore", func(t *testing.T) {
		name := "test-" + uuid.New().String()[:8]
		rootPath := filepath.Join("tmp", name)
		require.NoError(t, os.MkdirAll(rootPath, 0644))
		defer func() {
			require.NoError(t, os.RemoveAll(rootPath))
		}()
		specYaml := fmt.Sprintf(`build:
  name: %s`, name)
		dockerfile := `FROM alpine:3.11.6
COPY foo foo
COPY bar bar
CMD ["sh", "-c", "echo \"Hello, world\""]`
		require.NoError(t, createFiles(map[string]interface{}{
			"kindest.yaml":  specYaml,
			".dockerignore": "bar",
			"foo":           "hello, world!",
			"bar":           "this should be excluded!",
			"Dockerfile":    dockerfile,
		}, rootPath))
		p := NewProcess(logger.NewFakeLogger())
		module, err := p.GetModule(rootPath)
		require.NoError(t, err)
		err = module.Build(&BuildOptions{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "docker: COPY failed: stat ")
		require.Contains(t, err.Error(), "/bar: no such file or directory")
		require.Equal(t, BuildStatusFailed, module.Status())
	})
}

func TestModuleBuildCache(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0644))
	defer func() {
		require.NoError(t, os.RemoveAll(rootPath))
	}()
	specYaml := fmt.Sprintf(`build:
  name: %s`, name)
	dockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "echo \"Hello, world\""]`
	require.NoError(t, createFiles(map[string]interface{}{
		"kindest.yaml": specYaml,
		"Dockerfile":   dockerfile,
	}, rootPath))
	log := logger.NewMockLogger(logger.NewFakeLogger())
	module, err := NewProcess(log).GetModule(rootPath)
	require.NoError(t, err)
	require.Equal(t, BuildStatusPending, module.Status())
	require.NoError(t, module.Build(&BuildOptions{}))
	require.Equal(t, BuildStatusSucceeded, module.Status())
	require.False(t, log.WasObservedIgnoreFields("info", "No files changed"))
	module, err = NewProcess(log).GetModule(rootPath)
	require.NoError(t, err)
	require.Equal(t, BuildStatusPending, module.Status())
	require.NoError(t, module.Build(&BuildOptions{}))
	require.Equal(t, BuildStatusSucceeded, module.Status())
	require.True(t, log.WasObservedIgnoreFields("info", "No files changed"))
}

func TestModuleDependency(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0644))
	defer func() {
		require.NoError(t, os.RemoveAll(rootPath))
	}()
	depYaml := fmt.Sprintf(`build:
  name: %s-dep`, name)
	specYaml := fmt.Sprintf(`dependencies:
- dep
build:
  name: %s`, name)
	depDockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "echo \"Hello, world\""]`
	dockerfile := fmt.Sprintf(`FROM %s-dep:latest
CMD ["sh", "-c", "echo \"foo bar baz\""]`, name)
	require.NoError(t, createFiles(map[string]interface{}{
		"kindest.yaml": specYaml,
		"Dockerfile":   dockerfile,
		"dep": map[string]interface{}{
			"kindest.yaml": depYaml,
			"Dockerfile":   depDockerfile,
		},
	}, rootPath))
	module, err := NewProcess(logger.NewFakeLogger()).GetModule(rootPath)
	require.NoError(t, err)
	require.Equal(t, BuildStatusPending, module.Status())
	require.NoError(t, module.Build(&BuildOptions{}))
	require.Equal(t, BuildStatusSucceeded, module.Status())
}
