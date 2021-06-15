package main

import (
	"runtime"
	"time"

	"github.com/midcontinentcontrols/kindest/pkg/kindest"
	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

type BuildArgs struct {
	File        string `json:"file,omitempty" yaml:"file,omitempty"`
	Concurrency int    `json:"concurrency,omitempty" yaml:"concurrency,omitempty"`
	NoCache     bool   `json:"nocache,omitempty" yaml:"nocache,omitempty"`
	Squash      bool   `json:"squash,omitempty" yaml:"squash,omitempty"`
	Tag         string `json:"tag,omitempty" yaml:"tag,omitempty"`
	Builder     string `json:"builder,omitempty" yaml:"builder,omitempty"`
	Repository  string `json:"repository,omitempty" yaml:"repository,omitempty"`
	NoPush      bool   `json:"noPush,omitempty" yaml:"noPush,omitempty"`
	SkipHooks   bool   `json:"skipHooks,omitempty" yaml:"skipHooks,omitempty"`
	Verbose     bool   `json:"verbose,omitempty" yaml:"verbose,omitempty"`
	Force       bool   `json:"force,omitempty" yaml:"force,omitempty"`
}

var buildArgs BuildArgs

var buildCmd = &cobra.Command{
	Use: "build",
	RunE: func(cmd *cobra.Command, args []string) error {
		log := logger.NewZapLoggerFromEnv()
		module, err := kindest.NewProcess(
			buildArgs.Concurrency,
			log,
		).GetModule(buildArgs.File)
		if err != nil {
			return err
		}
		start := time.Now()
		if err := module.Build(&kindest.BuildOptions{
			NoCache:    buildArgs.NoCache,
			Squash:     buildArgs.Squash,
			Tag:        buildArgs.Tag,
			Builder:    buildArgs.Builder,
			Repository: buildArgs.Repository,
			NoPush:     buildArgs.NoPush,
			SkipHooks:  buildArgs.SkipHooks,
			Verbose:    buildArgs.Verbose,
			Force:      buildArgs.Force,
		}); err != nil {
			return err
		}
		log.Info("Build successful", zap.String("elapsed", time.Since(start).String()))
		return nil
	},
}

func init() {
	ConfigureCommand(buildCmd)
	buildCmd.PersistentFlags().StringVarP(&buildArgs.File, "file", "f", "./kindest.yaml", "Path to kindest.yaml file")
	buildCmd.PersistentFlags().IntVarP(&buildArgs.Concurrency, "concurrency", "c", runtime.NumCPU(), "number of parallel build jobs (defaults to num cpus)")
	buildCmd.PersistentFlags().BoolVar(&buildArgs.NoCache, "no-cache", false, "build images from scratch")
	buildCmd.PersistentFlags().StringVarP(&buildArgs.Tag, "tag", "t", "latest", "docker image tag")
	buildCmd.PersistentFlags().BoolVar(&buildArgs.Squash, "squash", false, "squashes newly built layers into a single new layer (docker experimental feature)")
	buildCmd.PersistentFlags().StringVar(&buildArgs.Builder, "builder", "docker", "builder backend (docker or kaniko)")
	buildCmd.PersistentFlags().BoolVar(&buildArgs.NoPush, "no-push", false, "do not push built images")
	buildCmd.PersistentFlags().StringVar(&buildArgs.Repository, "repository", "", "push repository override (e.g. localhost:5000)")
	buildCmd.PersistentFlags().BoolVar(&buildArgs.SkipHooks, "skip-hooks", false, "skip before: and after: hooks")
	buildCmd.PersistentFlags().BoolVarP(&buildArgs.Verbose, "verbose", "v", false, "verbose output (pipe build messages to stdout)")
	buildCmd.PersistentFlags().BoolVar(&buildArgs.Force, "force", false, "build regardless of digest")
}
