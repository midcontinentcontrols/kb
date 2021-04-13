package main

import (
	"runtime"

	"github.com/spf13/cobra"
)

type DeployArgs struct {
	File        string `json:"file,omitempty" yaml:"file,omitempty"`
	Concurrency int    `json:"concurrency,omitempty" yaml:"concurrency,omitempty"`
	NoCache     bool   `json:"nocache,omitempty" yaml:"nocache,omitempty"`
	Squash      bool   `json:"squash,omitempty" yaml:"squash,omitempty"`
	Tag         string `json:"tag,omitempty" yaml:"tag,omitempty"`
	Builder     string `json:"builder,omitempty" yaml:"builder,omitempty"`
	Repository  string `json:"repository,omitempty" yaml:"repository,omitempty"`
	NoPush      bool   `json:"noPush,omitempty" yaml:"noPush,omitempty"`
	SkipHooks   bool   `json:"skipHooks,omitempty" yaml:"skipHooks,omitempty"`
}

var deployArgs DeployArgs

var deployCmd = &cobra.Command{
	Use: "deploy",
	RunE: func(cmd *cobra.Command, args []string) error {
		/*
			log := logger.NewZapLoggerFromEnv()
			module, err := kindest.NewProcess(
				deployArgs.Concurrency,
				log,
			).GetModule(deployArgs.File)
			if err != nil {
				return err
			}
			start := time.Now()
			if err := module.Deploy(&kindest.DeployOptions{
				NoCache:    deployArgs.NoCache,
				Squash:     deployArgs.Squash,
				Tag:        deployArgs.Tag,
				Builder:    deployArgs.Builder,
				Repository: deployArgs.Repository,
				NoPush:     deployArgs.NoPush,
				SkipHooks:  deployArgs.SkipHooks,
			}); err != nil {
				return err
			}
			log.Info("Deploy successful", zap.String("elapsed", time.Now().Sub(start).String()))
		*/
		return nil
	},
}

func init() {
	ConfigureCommand(deployCmd)
	deployCmd.PersistentFlags().StringVarP(&deployArgs.File, "file", "f", "./kindest.yaml", "Path to kindest.yaml file")
	deployCmd.PersistentFlags().IntVarP(&deployArgs.Concurrency, "concurrency", "c", runtime.NumCPU(), "number of parallel build jobs (defaults to num cpus)")
	deployCmd.PersistentFlags().BoolVar(&deployArgs.NoCache, "no-cache", false, "build images from scratch")
	deployCmd.PersistentFlags().StringVarP(&deployArgs.Tag, "tag", "t", "latest", "docker image tag")
	deployCmd.PersistentFlags().BoolVar(&deployArgs.Squash, "squash", false, "squashes newly built layers into a single new layer (docker experimental feature)")
	deployCmd.PersistentFlags().StringVar(&deployArgs.Builder, "builder", "docker", "builder backend (docker or kaniko)")
	deployCmd.PersistentFlags().BoolVar(&deployArgs.NoPush, "no-push", false, "do not push built images")
	deployCmd.PersistentFlags().StringVar(&deployArgs.Repository, "repository", "", "push repository override (e.g. localhost:5000)")
	deployCmd.PersistentFlags().BoolVar(&deployArgs.SkipHooks, "skip-hooks", false, "skip before: and after: hooks")
	//deployCmd.PersistentFlags().StringVar(&deployArgs.Context, "context", "", "kubecontext (on-cluster build)")
	//deployCmd.PersistentFlags().StringVar(&deployArgs.Kind, "kind", "", "copy image to kind cluster instead of pushing")
}
