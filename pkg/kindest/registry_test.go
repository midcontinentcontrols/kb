package kindest

import (
	"context"
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
