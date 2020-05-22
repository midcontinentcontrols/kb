package kindest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Jeffail/tunny"
	"github.com/docker/docker/client"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newCLI(t *testing.T) client.APIClient {
	cli, err := client.NewEnvClient()
	require.NoError(t, err)
	return cli
}

func createBasicTestProject(t *testing.T, path string) string {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join(path, name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	dockerfile := `FROM alpine:latest
CMD ["sh", "-c", "echo \"Hello, world\""]`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`build:
  name: test/%s
  docker: {}
`, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	return specPath
}

func TestBuildBasic(t *testing.T) {
	specPath := createBasicTestProject(t, "tmp")
	defer os.RemoveAll(filepath.Dir(specPath))
	require.NoError(t, Build(
		&BuildOptions{
			File: specPath,
		},
		newCLI(t),
	))
}

func TestBuildErrDependencyBuildFailure(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := `FROM alpine:latest
CMD ["sh", "-c", "echo \"Hello, world\""]`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`dependencies: ["dep"]
build:
  name: test/%s
  docker: {}`, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	depPath := filepath.Join(rootPath, "dep")
	require.NoError(t, os.MkdirAll(depPath, 0766))
	depSpec := fmt.Sprintf(`
build:
  name: test/%s-dep
  docker: {}`, name)
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(depPath, "kindest.yaml"),
		[]byte(depSpec),
		0644,
	))
	depDockerfile := `FROM alpine:latest
RUN exit 1`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(depPath, "Dockerfile"),
		[]byte(depDockerfile),
		0644,
	))
	err := Build(
		&BuildOptions{
			File: specPath,
		},
		newCLI(t),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "dependency 'dep': The command '/bin/sh -c exit 1' returned a non-zero code: 1")
}

func TestBuildErrMissingDependencySpec(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := `FROM alpine:latest
CMD ["sh", "-c", "echo \"Hello, world\""]`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`dependencies: ["dep"]
build:
  name: test/%s
  docker: {}`, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	err := Build(
		&BuildOptions{
			File: specPath,
		},
		newCLI(t),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "dependency 0: missing kindest.yaml")
}

func TestBuildErrMissingDockerfile(t *testing.T) {
	specPath := createBasicTestProject(t, "tmp")
	rootPath := filepath.Dir(specPath)
	defer os.RemoveAll(rootPath)
	require.NoError(t, os.Remove(filepath.Join(rootPath, "Dockerfile")))
	err := Build(
		&BuildOptions{
			File: specPath,
		},
		newCLI(t),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing Dockerfile")
}

func TestBuildCustomDockerfilePath(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	subdir := filepath.Join(rootPath, "subdir")
	require.NoError(t, os.MkdirAll(subdir, 0766))
	dockerfile := `FROM alpine:latest
CMD ["sh", "-c", "echo \"Hello, world\""]`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(subdir, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`build:
  name: test/%s
  docker:
    dockerfile: subdir/Dockerfile
`, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	require.NoError(t, Build(
		&BuildOptions{File: specPath},
		newCLI(t),
	))
}

func TestBuildErrMissingBuildSpec(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	subdir := filepath.Join(rootPath, "subdir")
	require.NoError(t, os.MkdirAll(subdir, 0766))
	dockerfile := `FROM alpine:latest
CMD ["sh", "-c", "echo \"Hello, world\""]`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(subdir, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`build:
  name: test/%s
`, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	require.Error(t, ErrMissingBuildSpec, Build(
		&BuildOptions{File: specPath},
		newCLI(t),
	))
}

func TestBuildErrMissingName(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte("FROM alpine:latest"),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`build:
  docker: {}
`)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	require.Equal(t, ErrMissingImageName, Build(
		&BuildOptions{File: specPath},
		newCLI(t),
	))
}

func TestBuildErrMissingBuildArg(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := `FROM alpine:latest
ARG HAS_BUILD_ARG
RUN if [ -z "$HAS_BUILD_ARG" ]; then exit 1; fi`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`build:
  name: test/%s
  docker: {}
`, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	require.Error(t, Build(
		&BuildOptions{File: specPath},
		newCLI(t),
	))
}

func TestBuildArg(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := `FROM alpine:latest
ARG HAS_BUILD_ARG
RUN if [ -z "$HAS_BUILD_ARG" ]; then exit 1; fi`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`build:
  docker:
    name: test/%s
	buildArgs:
	  - name: HAS_BUILD_ARG
	    value: "1"
`, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	require.Error(t, Build(
		&BuildOptions{File: specPath},
		newCLI(t),
	))
}

func TestBuildContextPath(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	subdir := filepath.Join(rootPath, "subdir")
	otherdir := filepath.Join(rootPath, "other")
	require.NoError(t, os.MkdirAll(subdir, 0766))
	require.NoError(t, os.MkdirAll(otherdir, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := `FROM alpine:latest
CMD ["sh", "-c", "echo \"Hello, world\""]`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(otherdir, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	specPath := filepath.Join(otherdir, "kindest.yaml")
	spec := fmt.Sprintf(`build:
  name: test/%s
  docker:
    dockerfile: "../other/Dockerfile"
    context: ".."
`, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	require.NoError(t, Build(
		&BuildOptions{File: specPath},
		newCLI(t),
	))
}

func TestBuildDependency(t *testing.T) {
	depName := "test-" + uuid.New().String()[:8]
	name := "test-" + uuid.New().String()[:8]
	log.Info("Building dep test", zap.String("depName", depName), zap.String("name", name))
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	// Use the dependency as a base image
	dockerfile := fmt.Sprintf(`FROM test/%s:latest
CMD ["sh", "-c", "echo \"Hello again, world\""]`, depName)
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`dependencies:
  - dep
build:
  name: test/%s
  docker: {}
`, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	depDockerfile := `FROM alpine:latest
CMD ["sh", "-c", "echo \"Hello, world\""]`
	depPath := filepath.Join(rootPath, "dep")
	require.NoError(t, os.MkdirAll(depPath, 0766))
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(depPath, "Dockerfile"),
		[]byte(depDockerfile),
		0644,
	))
	depSpec := fmt.Sprintf(`build:
  name: test/%s
  docker: {}
`, depName)
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(depPath, "kindest.yaml"),
		[]byte(depSpec),
		0644,
	))
	require.NoError(t, Build(
		&BuildOptions{
			File: specPath,
		},
		newCLI(t),
	))
}

func TestBuildDependencyModule(t *testing.T) {
	depName := "test-" + uuid.New().String()[:8]
	name := "test-" + uuid.New().String()[:8]
	log.Info("Building dep test", zap.String("depName", depName), zap.String("name", name))
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	// Use the dependency as a base image
	dockerfile := fmt.Sprintf(`FROM test/%s:latest
CMD ["sh", "-c", "echo \"Hello again, world\""]`, depName)
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(`dependencies: ["dep"]`),
		0644,
	))
	depDockerfile := `FROM alpine:latest
CMD ["sh", "-c", "echo \"Hello, world\""]`
	depPath := filepath.Join(rootPath, "dep")
	require.NoError(t, os.MkdirAll(depPath, 0766))
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(depPath, "Dockerfile"),
		[]byte(depDockerfile),
		0644,
	))
	depSpec := fmt.Sprintf(`build:
  name: test/%s
  docker: {}
`, depName)
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(depPath, "kindest.yaml"),
		[]byte(depSpec),
		0644,
	))
	require.NoError(t, Build(
		&BuildOptions{
			File: specPath,
		},
		newCLI(t),
	))
}

func TestBuildCache(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := `FROM alpine:latest
RUN echo 'Hello, world!' >> /message
CMD ["cat", "/message"]`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`build:
  name: test/%s
  docker: {}
`, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	cli := newCLI(t)
	var pool *tunny.Pool
	var isUsingCache int32
	pool = tunny.NewFunc(runtime.NumCPU(), func(payload interface{}) interface{} {
		options := payload.(*BuildOptions)
		return BuildEx(options, cli, pool, func(r io.ReadCloser) error {
			rd := bufio.NewReader(r)
			for {
				message, err := rd.ReadString('\n')
				if err != nil {
					require.True(t, err == io.EOF)
					break
				}
				var msg struct {
					Stream string `json:"stream"`
				}
				require.NoError(t, json.Unmarshal([]byte(message), &msg))
				log.Info("Docker", zap.String("message", msg.Stream))
				if strings.Contains(msg.Stream, "Using cache") {
					atomic.StoreInt32(&isUsingCache, 1)
				}
			}
			return nil
		})
	})
	defer pool.Close()
	err, _ := pool.Process(&BuildOptions{File: specPath}).(error)
	require.NoError(t, err)
	err, _ = pool.Process(&BuildOptions{File: specPath}).(error)
	require.NoError(t, err)
	require.Equal(t, int32(1), atomic.LoadInt32(&isUsingCache))
}
