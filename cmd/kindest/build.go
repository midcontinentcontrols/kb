package main

import (
	"github.com/docker/docker/client"
	"github.com/midcontinentcontrols/kindest/pkg/kindest"
	"github.com/spf13/cobra"
)

var buildArgs kindest.BuildOptions

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "",
	RunE: func(cmd *cobra.Command, args []string) error {
		cli, err := client.NewEnvClient()
		if err != nil {
			return err
		}
		return kindest.Build(&buildArgs, cli)
	},
}

func init() {
	ConfigureCommand(buildCmd)
	buildCmd.PersistentFlags().StringVarP(&buildArgs.File, "file", "f", "./kindest.yaml", "Path to kindest.yaml file")
	buildCmd.PersistentFlags().StringVarP(&buildArgs.Tag, "tag", "t", "latest", "docker image tag")
	buildCmd.PersistentFlags().BoolVar(&buildArgs.NoCache, "no-cache", false, "build images from scratch")
	buildCmd.PersistentFlags().BoolVar(&buildArgs.Squash, "squash", false, "squashes newly built layers into a single new layer (docker experimental feature)")
}
