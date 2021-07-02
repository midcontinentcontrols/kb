package kindest

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"github.com/midcontinentcontrols/kindest/pkg/test"

	"github.com/stretchr/testify/require"
)

func TestModuleBuildStatus(t *testing.T) {
	t.Run("BuildStatusSucceeded", func(t *testing.T) {
		test.WithTemporaryModule(t, func(name string, rootPath string) {
			specYaml := fmt.Sprintf(`build:
  name: %s`, name)
			dockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "echo \"Hello, world\""]`
			require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
				"kindest.yaml": specYaml,
				"Dockerfile":   dockerfile,
			}))
			p := NewProcess(runtime.NumCPU(), logger.NewFakeLogger())
			module, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
			require.NoError(t, err)
			require.Equal(t, BuildStatusPending, module.Status())
			require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
			require.Equal(t, BuildStatusSucceeded, module.Status())
		})
	})
	t.Run("BuildStatusFailed", func(t *testing.T) {
		test.WithTemporaryModule(t, func(name string, rootPath string) {
			specYaml := fmt.Sprintf(`build:
  name: %s`, name)
			dockerfile := `FROM alpine:3.11.6
RUN cat foo`
			require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
				"kindest.yaml": specYaml,
				"Dockerfile":   dockerfile,
			}))
			p := NewProcess(runtime.NumCPU(), logger.NewFakeLogger())
			module, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
			require.NoError(t, err)
			err = module.Build(&BuildOptions{NoPush: true})
			require.Error(t, err)
			require.Contains(t, err.Error(), "The command '/bin/sh -c cat foo' returned a non-zero code: 1")
			require.Equal(t, BuildStatusFailed, module.Status())
		})
	})

	t.Run("ErrMissingDockerfile", func(t *testing.T) {
		test.WithTemporaryModule(t, func(name string, rootPath string) {
			specYaml := fmt.Sprintf(`build:
  name: %s`, name)
			require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
				"kindest.yaml": specYaml,
			}))
			p := NewProcess(runtime.NumCPU(), logger.NewFakeLogger())
			_, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
			require.Error(t, err)
			require.Contains(t, err.Error(), "missing Dockerfile")
		})
	})

	t.Run("ErrMissingName", func(t *testing.T) {
		test.WithTemporaryModule(t, func(name string, rootPath string) {
			dockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "echo \"Hello, world\""]`
			require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
				"kindest.yaml": "build: {}",
				"Dockerfile":   dockerfile,
			}))
			p := NewProcess(runtime.NumCPU(), logger.NewFakeLogger())
			_, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
			require.Error(t, err)
			require.Equal(t, err, ErrMissingImageName)
		})
	})
}

func TestModuleBuildContext(t *testing.T) {
	//
	// A basic test with a file copied over.
	t.Run("basic", func(t *testing.T) {
		test.WithTemporaryModule(t, func(name string, rootPath string) {
			specYaml := fmt.Sprintf(`build:
  name: %s`, name)
			dockerfile := `FROM alpine:3.11.6
COPY foo foo
RUN cat foo
CMD ["sh", "-c", "echo \"Hello, world\""]`
			require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
				"kindest.yaml": specYaml,
				"foo":          "hello, world!",
				"Dockerfile":   dockerfile,
			}))
			p := NewProcess(runtime.NumCPU(), logger.NewFakeLogger())
			module, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
			require.NoError(t, err)
			require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
			require.Equal(t, BuildStatusSucceeded, module.Status())
		})
	})

	//
	// Make sure dot syntax works for COPY/ADD directives
	t.Run("dot", func(t *testing.T) {
		test.WithTemporaryModule(t, func(name string, rootPath string) {
			specYaml := fmt.Sprintf(`build:
  name: %s`, name)
			dockerfile := `FROM alpine:3.11.6
COPY . .
RUN cat foo
CMD ["sh", "-c", "echo \"Hello, world\""]`
			require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
				"kindest.yaml": specYaml,
				"foo":          "hello, world!",
				"Dockerfile":   dockerfile,
			}))
			p := NewProcess(runtime.NumCPU(), logger.NewFakeLogger())
			module, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
			require.NoError(t, err)
			require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
			require.Equal(t, BuildStatusSucceeded, module.Status())
		})
	})

	//
	// This test ensures subdirectories are copied over correctly.
	t.Run("subdir", func(t *testing.T) {
		test.WithTemporaryModule(t, func(name string, rootPath string) {
			specYaml := fmt.Sprintf(`build:
  name: %s`, name)
			dockerfile := `FROM alpine:3.11.6
COPY subdir/foo subdir/foo
RUN cat subdir/foo
CMD ["sh", "-c", "echo \"Hello, world\""]`
			require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
				"kindest.yaml": specYaml,
				"subdir": map[string]interface{}{
					"foo": "hello, world!",
				},
				"Dockerfile": dockerfile,
			}))
			p := NewProcess(runtime.NumCPU(), logger.NewFakeLogger())
			module, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
			require.NoError(t, err)
			require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
			require.Equal(t, BuildStatusSucceeded, module.Status())
		})
	})

	//
	// This test ensure .dockerignore correctly excludes a single file.
	t.Run("dockerignore", func(t *testing.T) {
		test.WithTemporaryModule(t, func(name string, rootPath string) {
			specYaml := fmt.Sprintf(`build:
  name: %s
  context: ..`, name)
			dockerfile := `FROM alpine:3.11.6
COPY . .
RUN cat .git/foo
CMD ["sh", "-c", "echo \"Hello, world\""]`
			require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
				".dockerignore": ".git/",
				".git":          map[string]interface{}{"foo": "bar"},
				"dir": map[string]interface{}{
					"kindest.yaml": specYaml,
					"Dockerfile":   dockerfile,
				},
			}))
			p := NewProcess(runtime.NumCPU(), logger.NewFakeLogger())
			module, err := p.GetModule(filepath.Join(rootPath, "dir", "kindest.yaml"))
			require.NoError(t, err)
			err = module.Build(&BuildOptions{NoPush: true})
			require.Error(t, err)
			require.Contains(t, err.Error(), "docker: The command '/bin/sh -c cat .git/foo' returned a non-zero code: 1")
			require.Equal(t, BuildStatusFailed, module.Status())
		})
	})

	//
	// This test ensures parent directories can be used as build contexts.
	t.Run("parent", func(t *testing.T) {
		test.WithTemporaryModule(t, func(name string, rootPath string) {
			specYaml := fmt.Sprintf(`build:
  name: %s
  context: ../`, name)
			dockerfile := `FROM alpine:3.11.6
COPY foo foo
RUN cat foo
CMD ["sh", "-c", "echo \"Hello, world\""]`
			require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
				"subdir": map[string]interface{}{
					"kindest.yaml": specYaml,
					"Dockerfile":   dockerfile,
				},
				"foo": "hello, world!",
			}))
			p := NewProcess(runtime.NumCPU(), logger.NewFakeLogger())
			module, err := p.GetModule(filepath.Join(rootPath, "subdir", "kindest.yaml"))
			require.NoError(t, err)
			require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
			require.Equal(t, BuildStatusSucceeded, module.Status())
		})
	})

	//
	// This test ensures the contents of deeply nested directories are copied over.
	t.Run("deep", func(t *testing.T) {
		test.WithTemporaryModule(t, func(name string, rootPath string) {
			specYaml := fmt.Sprintf(`build:
  name: %s
  context: ../`, name)
			dockerfile := `FROM alpine:3.11.6
COPY subdir subdir
RUN cat subdir/deep/foo
CMD ["sh", "-c", "echo \"Hello, world\""]`
			require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
				"subdir": map[string]interface{}{
					"kindest.yaml": specYaml,
					"Dockerfile":   dockerfile,
					"deep": map[string]interface{}{
						"foo": "hello, world!",
					},
				},
			}))
			p := NewProcess(runtime.NumCPU(), logger.NewFakeLogger())
			module, err := p.GetModule(filepath.Join(rootPath, "subdir", "kindest.yaml"))
			require.NoError(t, err)
			require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
			require.Equal(t, BuildStatusSucceeded, module.Status())
		})
	})
}

//
// This test ensures the build cache is used when building an unchanged module.
func TestModuleBuildCache(t *testing.T) {
	test.WithTemporaryModule(t, func(name string, rootPath string) {
		specYaml := fmt.Sprintf(`build:
  name: %s`, name)
		dockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "echo \"Hello, world\""]`
		require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
			"kindest.yaml": specYaml,
			"Dockerfile":   dockerfile,
		}))
		log := logger.NewMockLogger(logger.NewFakeLogger())
		module, err := NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
		require.NoError(t, err)
		require.Equal(t, BuildStatusPending, module.Status())
		require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
		require.Equal(t, BuildStatusSucceeded, module.Status())
		require.False(t, log.WasObservedIgnoreFields("debug", "No files changed"))
		module, err = NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
		require.NoError(t, err)
		require.Equal(t, BuildStatusPending, module.Status())
		require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
		require.Equal(t, BuildStatusSucceeded, module.Status())
		require.True(t, log.WasObservedIgnoreFields("debug", "No files changed"))
	})
}

//
// This ensures a submodule can be included as a dependency, which is built
// on change along with the main module.
func TestModuleDependency(t *testing.T) {
	//
	// Ensure dependencies are cached along with the module being built.
	t.Run("cache", func(t *testing.T) {
		test.WithTemporaryModule(t, func(name string, rootPath string) {
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
			require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
				"kindest.yaml": specYaml,
				"Dockerfile":   dockerfile,
				"dep": map[string]interface{}{
					"kindest.yaml": depYaml,
					"Dockerfile":   depDockerfile,
				},
			}))
			log := logger.NewMockLogger(logger.NewFakeLogger())
			module, err := NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
			require.NoError(t, err)
			require.Equal(t, BuildStatusPending, module.Status())
			require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
			require.Equal(t, BuildStatusSucceeded, module.Status())
			require.True(t, log.WasObservedIgnoreFields("debug", "Digests do not match, building..."))
			require.False(t, log.WasObservedIgnoreFields("debug", "No files changed"))
			// Ensure the dep was cached
			log = logger.NewMockLogger(logger.NewFakeLogger())
			module, err = NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "dep", "kindest.yaml"))
			require.NoError(t, err)
			require.Equal(t, BuildStatusPending, module.Status())
			require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
			require.Equal(t, BuildStatusSucceeded, module.Status())
			require.False(t, log.WasObservedIgnoreFields("debug", "Digests do not match, building..."))
			require.True(t, log.WasObservedIgnoreFields("debug", "No files changed"))
		})
	})

	//
	// Ensure ListImages recursively returns dependenciess and
	// BuiltImages correctly reflects which images were changed.
	t.Run("list images", func(t *testing.T) {
		test.WithTemporaryModule(t, func(name string, rootPath string) {
			tag := uuid.New().String()[:8]
			depYaml := fmt.Sprintf(`build:
  name: %s-dep`, name)
			specYaml := fmt.Sprintf(`dependencies:
- dep
build:
  name: %s`,
				name)
			depDockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "echo \"Hello, world\""]`
			dockerfile := fmt.Sprintf(`ARG KINDEST_TAG
FROM %s-dep:${KINDEST_TAG}
COPY foo foo
CMD ["sh", "-c", "cat foo"]`, name)
			require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
				"kindest.yaml": specYaml,
				"Dockerfile":   dockerfile,
				"foo":          "hello world",
				"dep": map[string]interface{}{
					"kindest.yaml": depYaml,
					"Dockerfile":   depDockerfile,
				},
			}))
			log := logger.NewMockLogger(logger.NewFakeLogger())
			module, err := NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
			require.NoError(t, err)
			images, err := module.ListImages()
			require.NoError(t, err)
			require.Len(t, images, 2)
			require.NotEqual(t, images[0], images[1])
			require.Contains(t, images, name)
			require.Contains(t, images, name+"-dep")
			require.Equal(t, BuildStatusPending, module.Status())
			require.NoError(t, module.Build(&BuildOptions{
				NoPush: true,
				Tag:    tag,
			}))
			require.Equal(t, BuildStatusSucceeded, module.Status())
			require.False(t, log.WasObservedIgnoreFields("debug", "No files changed"))
			images = module.BuiltImages
			require.Len(t, images, 2)
			require.NotEqual(t, images[0], images[1])
			require.Contains(t, images, name+":"+tag)
			require.Contains(t, images, name+"-dep:"+tag)
			require.NoError(t, ioutil.WriteFile(
				filepath.Join(rootPath, "foo"),
				[]byte("this is updated"),
				0644,
			))
			module, err = NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
			require.NoError(t, err)
			require.NoError(t, module.Build(&BuildOptions{
				NoPush: true,
				Tag:    tag,
			}))
			require.Len(t, module.BuiltImages, 1)
			require.Equal(t, name+":"+tag, module.BuiltImages[0])

			// Update dep dockerfile and ensure BOTH are rebuilt
			// because the dep is the base for the main dockerfile
			depDockerfilePath, err := filepath.Abs(filepath.Join(rootPath, "dep", "Dockerfile"))
			require.NoError(t, err)
			depDockerfile = `FROM alpine:3.11.6
CMD ["sh", "-c", "echo \"it is updated, hooray\""]`
			require.NoError(t, ioutil.WriteFile(
				depDockerfilePath,
				[]byte(depDockerfile),
				0644,
			))
			module, err = NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
			require.NoError(t, err)
			affected, err := module.GetAffectedModules([]string{depDockerfilePath})
			require.NoError(t, err)
			require.Len(t, affected, 2)
			require.NoError(t, module.Build(&BuildOptions{
				NoPush: true,
				Tag:    tag,
			}))
			require.Len(t, module.BuiltImages, 2)
			require.Contains(t, module.BuiltImages, name+":"+tag)
			require.Contains(t, module.BuiltImages, name+"-dep:"+tag)
		})
	})

	//
	// Ensure errors in building a dependency are correctly propogated.
	t.Run("error", func(t *testing.T) {
		test.WithTemporaryModule(t, func(name string, rootPath string) {
			depYaml := fmt.Sprintf(`build:
  name: %s-dep`, name)
			specYaml := fmt.Sprintf(`dependencies:
- myDependency
build:
  name: %s`, name)
			depDockerfile := `FROM alpine:3.11.6
RUN cat /nonexistent
CMD ["sh", "-c", "echo \"Hello, world\""]`
			dockerfile := fmt.Sprintf(`FROM %s-dep:latest
CMD ["sh", "-c", "echo \"foo bar baz\""]`, name)
			require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
				"kindest.yaml": specYaml,
				"Dockerfile":   dockerfile,
				"myDependency": map[string]interface{}{
					"kindest.yaml": depYaml,
					"Dockerfile":   depDockerfile,
				},
			}))
			log := logger.NewMockLogger(logger.NewFakeLogger())
			module, err := NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
			require.NoError(t, err)
			require.Equal(t, BuildStatusPending, module.Status())
			err = module.Build(&BuildOptions{NoPush: true})
			require.Error(t, err)
			require.Contains(t, err.Error(), "myDependency")
			require.Contains(t, err.Error(), "The command '/bin/sh -c cat /nonexistent' returned a non-zero code: 1")
			require.Equal(t, BuildStatusFailed, module.Status())
		})
	})
}

//
// This ensures the buildArgs: section of kindest.yaml is properly applied
// when images are built.
func TestModuleBuildArgs(t *testing.T) {
	test.WithTemporaryModule(t, func(name string, rootPath string) {
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
		require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
			"kindest.yaml": specYaml,
			"Dockerfile":   dockerfile,
			"script":       script,
		}))
		p := NewProcess(runtime.NumCPU(), logger.NewFakeLogger())
		module, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
		require.NoError(t, err)
		require.Equal(t, BuildStatusPending, module.Status())
		require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
		require.Equal(t, BuildStatusSucceeded, module.Status())
	})
}

//
// These test ensure the various fields of the BuildOptions struct
// are correctly applied.
func TestModuleBuildOptions(t *testing.T) {
	//
	// Test the functionality of --no-cache
	t.Run("no cache", func(t *testing.T) {
		test.WithTemporaryModule(t, func(name string, rootPath string) {
			specYaml := fmt.Sprintf(`build:
  name: %s`, name)
			dockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "echo \"Hello, world\""]`
			require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
				"kindest.yaml": specYaml,
				"Dockerfile":   dockerfile,
			}))
			log := logger.NewMockLogger(logger.NewFakeLogger())
			module, err := NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
			require.NoError(t, err)
			require.Equal(t, BuildStatusPending, module.Status())
			require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
			require.Equal(t, BuildStatusSucceeded, module.Status())
			require.False(t, log.WasObservedIgnoreFields("debug", "No files changed"))
			module, err = NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
			require.NoError(t, err)
			require.Equal(t, BuildStatusPending, module.Status())
			require.NoError(t, module.Build(&BuildOptions{NoCache: true, NoPush: true}))
			require.Equal(t, BuildStatusSucceeded, module.Status())
			require.False(t, log.WasObservedIgnoreFields("debug", "No files changed"))
		})
	})

	//
	// Ensure custom tags (--tag or otherwise) are used when specified.
	// Note: the default tag is "latest"
	t.Run("tag", func(t *testing.T) {
		test.WithTemporaryModule(t, func(name string, rootPath string) {
			specYaml := fmt.Sprintf(`build:
  name: %s`, name)
			dockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "echo \"Hello, world\""]`
			require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
				"kindest.yaml": specYaml,
				"Dockerfile":   dockerfile,
			}))
			log := logger.NewMockLogger(logger.NewFakeLogger())
			module, err := NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
			require.NoError(t, err)
			require.Equal(t, BuildStatusPending, module.Status())
			tag := "foobar"
			require.NoError(t, module.Build(&BuildOptions{Tag: tag, NoPush: true}))
			require.Equal(t, BuildStatusSucceeded, module.Status())
			require.True(t, log.WasObservedIgnoreFields("info", "Successfully built image"))
		})
	})

	//
	// Test the functionality of --no-cache
	t.Run("repository", func(t *testing.T) {
		test.WithTemporaryModule(t, func(name string, rootPath string) {
			specYaml := fmt.Sprintf(`build:
  name: %s`, name)
			dockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "echo \"Hello, world\""]`
			require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
				"kindest.yaml": specYaml,
				"Dockerfile":   dockerfile,
			}))
			log := logger.NewMockLogger(logger.NewFakeLogger())
			module, err := NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
			require.NoError(t, err)
			require.Equal(t, BuildStatusPending, module.Status())
			require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
			require.Equal(t, BuildStatusSucceeded, module.Status())
			require.False(t, log.WasObservedIgnoreFields("debug", "No files changed"))
			module, err = NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
			require.NoError(t, err)
			require.Equal(t, BuildStatusPending, module.Status())
			require.NoError(t, module.Build(&BuildOptions{NoCache: true, NoPush: true}))
			require.Equal(t, BuildStatusSucceeded, module.Status())
			require.False(t, log.WasObservedIgnoreFields("debug", "No files changed"))
		})
	})
}
