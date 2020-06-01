package kindest

import (
	"context"
	"fmt"

	containertypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"go.uber.org/zap"
)

func CreateLocalRegistry(name string, port int, cli client.APIClient) error {
	portStr := fmt.Sprintf("%d", name)
	info, err := cli.ContainerCreate(
		context.TODO(),
		&containertypes.Config{
			Image: "registry:2",
		},
		&containertypes.HostConfig{
			RestartPolicy: containertypes.RestartPolicy{
				Name: "Always",
			},
			PortBindings: nat.PortMap(map[nat.Port][]nat.PortBinding{
				nat.Port(portStr): []nat.PortBinding{{
					HostPort: portStr,
				}},
			}),
		},
		nil,
		name,
	)
	if err != nil {
		return err
	}
	log := log.With(zap.String("id", info.ID))
	log.Info("Created registry container")
	for _, warning := range info.Warnings {
		log.Warn("Container create warning", zap.String("message", warning))
	}
	return nil
}

// EnsureRegistryRunning ensures a local docker registry is running,
// as per https://kind.sigs.k8s.io/docs/user/local-registry/
func EnsureRegistryRunning(cli client.APIClient) error {
	name := "kind-registry"
	port := 5000
	_, err := cli.ContainerInspect(context.TODO(), name)
	if err != nil {
		if client.IsErrNotFound(err) {
			return CreateLocalRegistry(name, port, cli)
		}
		return err
	}
	return nil
}
