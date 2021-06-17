package main

import (
	"fmt"

	"github.com/docker/docker/client"
	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"github.com/midcontinentcontrols/kindest/pkg/registry"
	"github.com/spf13/cobra"
)

type RegistryArgs struct {
	NoConnect bool
}

var registryArgs RegistryArgs

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Ensures local registry is started",
	RunE: func(cmd *cobra.Command, args []string) error {
		log := logger.NewZapLoggerFromEnv()
		cli, err := client.NewClientWithOpts(client.FromEnv)
		if err != nil {
			return fmt.Errorf("docker: %v", err)
		}
		if err := registry.EnsureLocalRegistryRunning(
			cli,
			!registryArgs.NoConnect,
			log,
		); err != nil {
			return fmt.Errorf("registry: %v", err)
		}
		return nil
	},
}

func init() {
	ConfigureCommand(registryCmd)
	registryCmd.PersistentFlags().BoolVar(&registryArgs.NoConnect, "NoConnect", false, "Do not connect to kind network")
}
