package kindest

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types"

	containertypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"go.uber.org/zap"
)

func CreateLocalRegistry(regName string, regPort int, cli client.APIClient) error {
	portStr := fmt.Sprintf("%d/tcp", regPort)
	info, err := cli.ContainerCreate(
		context.TODO(),
		&containertypes.Config{
			Image: "registry:2",
		},
		&containertypes.HostConfig{
			RestartPolicy: containertypes.RestartPolicy{
				Name: "always",
			},
			PortBindings: nat.PortMap(map[nat.Port][]nat.PortBinding{
				nat.Port(portStr): []nat.PortBinding{{
					HostIP:   "127.0.0.1",
					HostPort: portStr,
				}},
			}),
		},
		nil,
		regName,
	)
	if err != nil {
		return err
	}
	if err := waitForContainer(regName, cli); err != nil {
		return err
	}
	log := log.With(zap.String("id", info.ID))
	log.Info("Created registry container")
	for _, warning := range info.Warnings {
		log.Warn("Container create warning", zap.String("message", warning))
	}
	return nil
}

func DeleteRegistry(cli client.APIClient) error {
	name := "kind-registry"
	timeout := 10 * time.Second
	log := log.With(zap.String("name", name))
	log.Info("Stopping registry container")
	if err := cli.ContainerStop(
		context.TODO(),
		name,
		&timeout,
	); err != nil {
		return err
	}
	force := true
	log.Info("Removing registry container", zap.Bool("force", force))
	if err := cli.ContainerRemove(
		context.TODO(),
		name,
		types.ContainerRemoveOptions{
			Force: force,
		},
	); err != nil {
		return err
	}
	return nil
}

// EnsureRegistryRunning ensures a local docker registry is running,
// as per https://kind.sigs.k8s.io/docs/user/local-registry/
func EnsureRegistryRunning(cli client.APIClient) error {
	regName := "kind-registry"
	regPort := 5000
	if err := waitForContainer(regName, cli); err != nil {
		if client.IsErrNotFound(err) {
			return CreateLocalRegistry(regName, regPort, cli)
		}
		return err
	}
	return nil
}
