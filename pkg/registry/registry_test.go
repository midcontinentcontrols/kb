package registry

import (
	"context"
	"testing"

	"github.com/midcontinentcontrols/kindest/pkg/test"

	"github.com/docker/docker/client"

	"github.com/stretchr/testify/require"
)

func TestDockerContainerInspect(t *testing.T) {
	t.Run("ErrNotFound", func(t *testing.T) {
		cli := test.NewDockerClient(t)
		name := RandomTestName()
		_, err := cli.ContainerInspect(context.TODO(), name)
		require.True(t, client.IsErrNotFound(err))
	})
}

func TestLocalRegistryCreateDelete(t *testing.T) {
	var err error
	cli := test.NewDockerClient(t)
	log := test.NewTestLogger()
	// Successive calls to EnsureRegistryRunning should do nothing
	require.NoError(t, EnsureLocalRegistryRunning(cli, log))
	require.NoError(t, EnsureLocalRegistryRunning(cli, log))
	require.NoError(t, EnsureLocalRegistryRunning(cli, log))
	require.NoError(t, EnsureLocalRegistryRunning(cli, log))
	_, err = cli.ContainerInspect(context.TODO(), "kind-registry")
	require.NoError(t, err)
	require.NoError(t, DeleteLocalRegistry(cli, log))
	_, err = cli.ContainerInspect(context.TODO(), "kind-registry")
	require.True(t, client.IsErrNotFound(err))
}

/*
func TestLocalRegistryPullImage(t *testing.T) {
	name := RandomTestName()
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := fmt.Sprintf(`FROM alpine:3.11.6
CMD ["sh", "-c", "echo \"Hello world!\""]`)
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`build:
  name: test/%s
test:
  - name: basic
    env:
      kubernetes: {}
    build:
      name: test/%s-test
      dockerfile: Dockerfile
`, name, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	require.NoError(t, Test(
		&TestOptions{
			File:       specPath,
			Transient:  true,
			Repository: "localhost:5000",
			NoRegistry: false,
		},
		logger.NewZapLoggerFromEnv(),
	))
}
*/
