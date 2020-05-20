package main

import (
	"runtime"

	"github.com/Jeffail/tunny"
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
		var pool *tunny.Pool
		pool = tunny.NewFunc(buildArgs.Concurrency, func(payload interface{}) interface{} {
			options := payload.(*kindest.BuildOptions)
			return kindest.BuildWithPool(options, cli, pool)
		})
		defer pool.Close()
		err, _ = pool.Process(&buildArgs).(error)
		return err
	},
}

func init() {
	ConfigureCommand(buildCmd)
	buildCmd.PersistentFlags().StringVarP(&buildArgs.File, "file", "f", "./kindest.yaml", "Path to kindest.yaml file")
	buildCmd.PersistentFlags().StringVarP(&buildArgs.Tag, "tag", "t", "latest", "docker image tag")
	buildCmd.PersistentFlags().BoolVar(&buildArgs.NoCache, "no-cache", false, "build images from scratch")
	buildCmd.PersistentFlags().BoolVar(&buildArgs.Squash, "squash", false, "squashes newly built layers into a single new layer (docker experimental feature)")
	buildCmd.PersistentFlags().IntVarP(&buildArgs.Concurrency, "concurrency", "c", runtime.NumCPU(), "number of parallel build jobs (defaults to num cpus)")
}
