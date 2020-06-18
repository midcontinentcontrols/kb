package main

import (
	"runtime"
	"time"

	"github.com/Jeffail/tunny"
	"github.com/midcontinentcontrols/kindest/pkg/kindest"
	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var buildOptions kindest.BuildOptions

var buildCmd = &cobra.Command{
	Use: "build",
	RunE: func(cmd *cobra.Command, args []string) error {
		log := logger.NewZapLoggerFromEnv()
		start := time.Now()
		var pool *tunny.Pool
		pool = tunny.NewFunc(buildOptions.Concurrency, func(payload interface{}) interface{} {
			return kindest.BuildEx(
				payload.(*kindest.BuildOptions),
				pool,
				nil,
				nil,
				log,
			)
		})
		defer pool.Close()
		err, _ := pool.Process(&buildOptions).(error)
		if err != nil {
			return err
		}
		log.Info("Build successful", zap.String("elapsed", time.Now().Sub(start).String()))
		return nil
	},
}

func init() {
	ConfigureCommand(buildCmd)
	buildCmd.PersistentFlags().StringVarP(&buildOptions.File, "file", "f", "./kindest.yaml", "Path to kindest.yaml file")
	buildCmd.PersistentFlags().BoolVar(&buildOptions.NoCache, "no-cache", false, "build images from scratch")
	buildCmd.PersistentFlags().StringVarP(&buildOptions.Tag, "tag", "t", "latest", "docker image tag")
	buildCmd.PersistentFlags().BoolVar(&buildOptions.Squash, "squash", false, "squashes newly built layers into a single new layer (docker experimental feature)")
	buildCmd.PersistentFlags().IntVarP(&buildOptions.Concurrency, "concurrency", "c", runtime.NumCPU(), "number of parallel build jobs (defaults to num cpus)")
	buildCmd.PersistentFlags().StringVar(&buildOptions.Context, "context", "", "kubecontext (on-cluster build)")
	buildCmd.PersistentFlags().StringVar(&buildOptions.Builder, "builder", "docker", "builder backend (docker or kaniko)")
	buildCmd.PersistentFlags().BoolVar(&buildOptions.NoPush, "no-push", false, "do not push built images")
	buildCmd.PersistentFlags().StringVar(&buildOptions.Kind, "kind", "", "copy image to kind cluster instead of pushing")
	testCmd.PersistentFlags().StringVar(&buildOptions.Repository, "repository", "", "push repository override (e.g. localhost:5000)")
}
