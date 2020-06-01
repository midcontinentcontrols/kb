package kindest

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/docker/client"

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

func TestRegistryCreateDelete(t *testing.T) {
	var err error
	cli := newCLI(t)
	// Successive calls to EnsureRegistryRunning should do nothing
	require.NoError(t, EnsureRegistryRunning(cli))
	require.NoError(t, EnsureRegistryRunning(cli))
	require.NoError(t, EnsureRegistryRunning(cli))
	require.NoError(t, EnsureRegistryRunning(cli))
	_, err = cli.ContainerInspect(context.TODO(), "kind-registry")
	require.NoError(t, err)
	require.NoError(t, DeleteRegistry(cli))
	_, err = cli.ContainerInspect(context.TODO(), "kind-registry")
	require.True(t, client.IsErrNotFound(err))
}

func TestRegistryPullImage(t *testing.T) {
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
			NoRegistry: false,
		},
	))
}
