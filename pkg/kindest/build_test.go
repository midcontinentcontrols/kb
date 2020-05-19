package kindest

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestBuildBasic(t *testing.T) {
	name := "test/test-" + uuid.New().String()[:8]
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
	require.NoError(t, Build(
		&KindestSpec{
			Name: name,
			Build: BuildSpec{
				Docker: &DockerBuildSpec{},
			},
		},
		rootPath,
		"latest",
		false,
		false,
	))
}

func TestBuildCustomDockerfilePath(t *testing.T) {
	name := "test/test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := `FROM alpine:latest
CMD ["sh", "-c", "echo \"Hello, world\""]`
	require.NoError(t, os.MkdirAll(filepath.Join(rootPath, "foo"), 0766))
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "foo", "bar"),
		[]byte(dockerfile),
		0644,
	))
	require.NoError(t, Build(
		&KindestSpec{
			Name: name,
			Build: BuildSpec{
				Docker: &DockerBuildSpec{
					Dockerfile: "foo/bar",
				},
			},
		},
		rootPath,
		"latest",
		false,
		false,
	))
}

func TestBuildErrMissingBuildArg(t *testing.T) {
	name := "test/test-" + uuid.New().String()[:8]
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
	require.Error(t, Build(
		&KindestSpec{
			Name: name,
			Build: BuildSpec{
				Docker: &DockerBuildSpec{},
			},
		},
		rootPath,
		"latest",
		false,
		false,
	))
}

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
