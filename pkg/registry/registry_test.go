package registry

import (
	"context"
	"testing"

	"github.com/midcontinentcontrols/kb/pkg/test"

	"github.com/docker/docker/client"

	"github.com/stretchr/testify/require"
)

func TestDockerContainerInspect(t *testing.T) {
	t.Run("ErrNotFound", func(t *testing.T) {
		cli := test.NewDockerClient(t)
		name := test.RandomTestName()
		_, err := cli.ContainerInspect(context.TODO(), name)
		require.True(t, client.IsErrNotFound(err))
	})
}

func TestLocalRegistryCreateDelete(t *testing.T) {
	var err error
	cli := test.NewDockerClient(t)
	log := test.NewTestLogger()
	// Successive calls to EnsureRegistryRunning should do nothing
	require.NoError(t, EnsureLocalRegistryRunning(cli, false, log))
	require.NoError(t, EnsureLocalRegistryRunning(cli, false, log))
	require.NoError(t, EnsureLocalRegistryRunning(cli, false, log))
	require.NoError(t, EnsureLocalRegistryRunning(cli, false, log))
	_, err = cli.ContainerInspect(context.TODO(), "kind-registry")
	require.NoError(t, err)
	require.NoError(t, DeleteLocalRegistry(cli, log))
	_, err = cli.ContainerInspect(context.TODO(), "kind-registry")
	require.True(t, client.IsErrNotFound(err))
}
