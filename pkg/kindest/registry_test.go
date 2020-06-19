package kindest

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/docker/client"
	"github.com/midcontinentcontrols/kindest/pkg/logger"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestDockerContainerInspect(t *testing.T) {
	t.Run("nonexistent", func(t *testing.T) {
		cli := newCLI(t)
		name := "test-" + uuid.New().String()[:8]
		_, err := cli.ContainerInspect(context.TODO(), name)
		require.True(t, client.IsErrNotFound(err))
	})
}

func TestLocalRegistryCreateDelete(t *testing.T) {
	var err error
	cli := newCLI(t)
	log := logger.NewFakeLogger()
	// Successive calls to EnsureRegistryRunning should do nothing
	require.NoError(t, EnsureLocalRegistryRunning(cli, log))
	require.NoError(t, EnsureLocalRegistryRunning(cli, log))
	require.NoError(t, EnsureLocalRegistryRunning(cli, log))
	require.NoError(t, EnsureLocalRegistryRunning(cli, log))
	_, err = cli.ContainerInspect(context.TODO(), "kind-registry")
	require.NoError(t, err)
	require.NoError(t, DeleteRegistry(cli, log))
	_, err = cli.ContainerInspect(context.TODO(), "kind-registry")
	require.True(t, client.IsErrNotFound(err))
}

func TestLocalRegistryPullImage(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
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

/*
func TestInClusterRegistryCreateDelete(t *testing.T) {
	transient := os.Getenv("KINDEST_PERSISTENT") != "1"
	kind := "kindest"
	provider := cluster.NewProvider()
	exists := false
	if !transient {
		clusters, err := provider.List()
		require.NoError(t, err)
		for _, cluster := range clusters {
			if cluster == kind {
				exists = true
				break
			}
		}
	} else {
		kind += "-" + uuid.New().String()[:8]
	}
	if !exists {
		require.NoError(t, provider.Create(kind))
	}
	if transient {
		defer func() {
			require.NoError(t, provider.Delete(kind, ""))
		}()
	}
	client, _, err := clientForKindCluster(kind, provider)
	log := logger.NewFakeLogger()
	require.NoError(t, err)
	require.NoError(t, waitForCluster(client, log))
	require.NoError(t, EnsureInClusterRegistryRunning(client, log))
	require.NoError(t, EnsureInClusterRegistryRunning(client, log))
	require.NoError(t, EnsureInClusterRegistryRunning(client, log))
	require.NoError(t, ensureDeployment(registryDeployment(), client, log))
	require.NoError(t, ensureService(registryService(), client, log))
}
*/
