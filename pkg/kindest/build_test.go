package kindest

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/docker/client"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func newCLI(t *testing.T) client.APIClient {
	cli, err := client.NewEnvClient()
	require.NoError(t, err)
	return cli
}

func TestBuildBasic(t *testing.T) {
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
	spec := fmt.Sprintf(`name: docker.io/test/%s
build:
  docker: {}
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
	spec := fmt.Sprintf(`name: docker.io/test/%s
build:
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
	spec := fmt.Sprintf(`name: docker.io/test/%s
build:
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

/*


func TestBuildArg(t *testing.T) {
	name := "test/test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := `FROM alpine:latest
ARG HAS_BUILD_ARG
RUN if [ -z "$HAS_BUILD_ARG" ]; then exit 1; fi
CMD ["sh", "-c", "echo \"Hello, world\""]`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	require.NoError(t, Build(
		&KindestSpec{
			Name: name,
			Build: BuildSpec{
				Docker: &DockerBuildSpec{
					BuildArgs: []*DockerBuildArg{{
						Name:  "HAS_BUILD_ARG",
						Value: "1",
					}},
				},
			},
		},
		rootPath,
		"latest",
		false,
		false,
	))
}

func TestBuildContextPath(t *testing.T) {
	repo := "test-" + uuid.New().String()[:8]
	name := "test/" + repo
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := fmt.Sprintf(`FROM alpine:latest
COPY %s .
CMD ["sh", "-c", "echo \"Hello, world\""]`, repo)
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	require.NoError(t, Build(
		&KindestSpec{
			Name: name,
			Build: BuildSpec{
				Docker: &DockerBuildSpec{
					Dockerfile: repo + "/Dockerfile",
				},
			},
		},
		filepath.Join(rootPath, ".."),
		"latest",
		false,
		false,
	))
}
*/
