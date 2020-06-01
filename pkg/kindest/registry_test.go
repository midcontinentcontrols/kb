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

func TestRegistryCreate(t *testing.T) {
	cli := newCLI(t)
	require.NoError(t, EnsureRegistryRunning(cli))
}
