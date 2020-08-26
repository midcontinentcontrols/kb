package kindest

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"go.uber.org/zap"

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
		p := NewProcess(runtime.NumCPU(), logger.NewFakeLogger())
		module, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
		require.NoError(t, err)
		require.Equal(t, BuildStatusPending, module.Status())
		require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
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
		p := NewProcess(runtime.NumCPU(), logger.NewFakeLogger())
		module, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
		require.NoError(t, err)
		err = module.Build(&BuildOptions{NoPush: true})
		require.Error(t, err)
		require.Contains(t, err.Error(), "docker: The command '/bin/sh -c cat foo' returned a non-zero code: 1")
		require.Equal(t, BuildStatusFailed, module.Status())
	})
}

func TestModuleBuildContext(t *testing.T) {
	//
	// A basic test with a file copied over.
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
		p := NewProcess(runtime.NumCPU(), logger.NewFakeLogger())
		module, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
		require.NoError(t, err)
		require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
		require.Equal(t, BuildStatusSucceeded, module.Status())
	})

	//
	// This test ensures subdirectories are copied over correctly.
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
		p := NewProcess(runtime.NumCPU(), logger.NewFakeLogger())
		module, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
		require.NoError(t, err)
		require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
		require.Equal(t, BuildStatusSucceeded, module.Status())
	})

	//
	// This test ensure .dockerignore correctly excludes a single file.
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
		p := NewProcess(runtime.NumCPU(), logger.NewFakeLogger())
		module, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
		require.NoError(t, err)
		err = module.Build(&BuildOptions{NoPush: true})
		require.Error(t, err)
		require.Contains(t, err.Error(), "docker: COPY failed: stat ")
		require.Contains(t, err.Error(), "/bar: no such file or directory")
		require.Equal(t, BuildStatusFailed, module.Status())
	})

	//
	// This test ensures parent directories can be used as build contexts.
	t.Run("parent", func(t *testing.T) {
		name := "test-" + uuid.New().String()[:8]
		rootPath := filepath.Join("tmp", name)
		require.NoError(t, os.MkdirAll(rootPath, 0644))
		defer func() {
			require.NoError(t, os.RemoveAll(rootPath))
		}()
		specYaml := fmt.Sprintf(`build:
  name: %s
  context: ../`, name)
		dockerfile := `FROM alpine:3.11.6
COPY foo foo
RUN cat foo
CMD ["sh", "-c", "echo \"Hello, world\""]`
		require.NoError(t, createFiles(map[string]interface{}{
			"subdir": map[string]interface{}{
				"kindest.yaml": specYaml,
				"Dockerfile":   dockerfile,
			},
			"foo": "hello, world!",
		}, rootPath))
		p := NewProcess(runtime.NumCPU(), logger.NewFakeLogger())
		module, err := p.GetModule(filepath.Join(rootPath, "subdir", "kindest.yaml"))
		require.NoError(t, err)
		require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
		require.Equal(t, BuildStatusSucceeded, module.Status())
	})
}

//
// This test ensures the build cache is used when building an unchanged module.
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
	module, err := NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
	require.NoError(t, err)
	require.Equal(t, BuildStatusPending, module.Status())
	require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
	require.Equal(t, BuildStatusSucceeded, module.Status())
	require.False(t, log.WasObservedIgnoreFields("info", "No files changed"))
	module, err = NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
	require.NoError(t, err)
	require.Equal(t, BuildStatusPending, module.Status())
	require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
	require.Equal(t, BuildStatusSucceeded, module.Status())
	require.True(t, log.WasObservedIgnoreFields("info", "No files changed"))
}

//
// This ensures a submodule can be included as a dependency, which is built
// on change along with the main module.
func TestModuleDependency(t *testing.T) {
	//
	// Ensure dependencies are cached along with the module being built.
	t.Run("cache", func(t *testing.T) {
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
		log := logger.NewMockLogger(logger.NewFakeLogger())
		module, err := NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
		require.NoError(t, err)
		require.Equal(t, BuildStatusPending, module.Status())
		require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
		require.Equal(t, BuildStatusSucceeded, module.Status())
		require.False(t, log.WasObservedIgnoreFields("info", "No files changed"))
		// Ensure the dep was cached
		module, err = NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "dep", "kindest.yaml"))
		require.NoError(t, err)
		require.Equal(t, BuildStatusPending, module.Status())
		require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
		require.Equal(t, BuildStatusSucceeded, module.Status())
		require.True(t, log.WasObservedIgnoreFields("info", "No files changed"))
	})

	//
	// Ensure errors in building a dependency are correctly propogated.
	t.Run("error", func(t *testing.T) {
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
RUN cat /nonexistent
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
		log := logger.NewMockLogger(logger.NewFakeLogger())
		module, err := NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
		require.NoError(t, err)
		require.Equal(t, BuildStatusPending, module.Status())
		err = module.Build(&BuildOptions{NoPush: true})
		require.Error(t, err)
		require.Contains(t, err.Error(), "/dep: docker: The command '/bin/sh -c cat /nonexistent' returned a non-zero code: 1")
		require.Equal(t, BuildStatusFailed, module.Status())
	})
}

//
// This ensures the buildArgs: section of kindest.yaml is properly applied
// when images are built.
func TestModuleBuildArgs(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0644))
	defer func() {
		require.NoError(t, os.RemoveAll(rootPath))
	}()
	script := `#!/bin/bash
set -euo pipefail
if [ "$foo" -ne "bar" ]; then
	exit 100
fi`
	specYaml := fmt.Sprintf(`build:
  name: %s
  buildArgs:
    - name: foo
      value: bar`, name)
	dockerfile := `FROM alpine:3.11.6
ARG foo
RUN apk add --no-cache bash
COPY script .
RUN echo "foo isss $foo"
RUN chmod +x ./script && ./script
CMD ["sh", "-c", "echo \"Hello, world\""]`
	require.NoError(t, createFiles(map[string]interface{}{
		"kindest.yaml": specYaml,
		"Dockerfile":   dockerfile,
		"script":       script,
	}, rootPath))
	p := NewProcess(runtime.NumCPU(), logger.NewFakeLogger())
	module, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
	require.NoError(t, err)
	require.Equal(t, BuildStatusPending, module.Status())
	require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
	require.Equal(t, BuildStatusSucceeded, module.Status())
}

//
// These test ensure the various fields of the BuildOptions struct
// are correctly applied.
func TestModuleBuildOptions(t *testing.T) {
	//
	// Test the functionality of --no-cache
	t.Run("no cache", func(t *testing.T) {
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
		module, err := NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
		require.NoError(t, err)
		require.Equal(t, BuildStatusPending, module.Status())
		require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
		require.Equal(t, BuildStatusSucceeded, module.Status())
		require.False(t, log.WasObservedIgnoreFields("info", "No files changed"))
		module, err = NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
		require.NoError(t, err)
		require.Equal(t, BuildStatusPending, module.Status())
		require.NoError(t, module.Build(&BuildOptions{NoCache: true, NoPush: true}))
		require.Equal(t, BuildStatusSucceeded, module.Status())
		require.False(t, log.WasObservedIgnoreFields("info", "No files changed"))
	})

	//
	// Ensure custom tags (--tag or otherwise) are used when specified.
	// Note: the default tag is "latest"
	t.Run("tag", func(t *testing.T) {
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
		module, err := NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
		require.NoError(t, err)
		require.Equal(t, BuildStatusPending, module.Status())
		tag := "foobar"
		require.NoError(t, module.Build(&BuildOptions{Tag: tag, NoPush: true}))
		require.Equal(t, BuildStatusSucceeded, module.Status())
		require.True(t, log.WasObserved("info", "Successfully built image", zap.String("dest", fmt.Sprintf("%s:%s", name, tag))))
	})

	//
	// Test the functionality of --no-cache
	t.Run("repository", func(t *testing.T) {
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
		module, err := NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
		require.NoError(t, err)
		require.Equal(t, BuildStatusPending, module.Status())
		require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
		require.Equal(t, BuildStatusSucceeded, module.Status())
		require.False(t, log.WasObservedIgnoreFields("info", "No files changed"))
		module, err = NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
		require.NoError(t, err)
		require.Equal(t, BuildStatusPending, module.Status())
		require.NoError(t, module.Build(&BuildOptions{NoCache: true, NoPush: true}))
		require.Equal(t, BuildStatusSucceeded, module.Status())
		require.False(t, log.WasObservedIgnoreFields("info", "No files changed"))
	})
}
