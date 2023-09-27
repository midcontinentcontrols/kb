package main

import (
	"runtime"
	"time"

	"github.com/midcontinentcontrols/kb/pkg/kb"
	"github.com/midcontinentcontrols/kb/pkg/logger"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

type BuildArgs struct {
	File        string   `json:"file,omitempty" yaml:"file,omitempty"`
	Concurrency int      `json:"concurrency,omitempty" yaml:"concurrency,omitempty"`
	NoCache     bool     `json:"nocache,omitempty" yaml:"nocache,omitempty"`
	Squash      bool     `json:"squash,omitempty" yaml:"squash,omitempty"`
	Tag         string   `json:"tag,omitempty" yaml:"tag,omitempty"`
	Builder     string   `json:"builder,omitempty" yaml:"builder,omitempty"`
	Repository  string   `json:"repository,omitempty" yaml:"repository,omitempty"`
	NoPush      bool     `json:"noPush,omitempty" yaml:"noPush,omitempty"`
	NoPushDeps  bool     `json:"noPushDeps,omitempty" yaml:"noPushDeps,omitempty"`
	SkipHooks   bool     `json:"skipHooks,omitempty" yaml:"skipHooks,omitempty"`
	Verbose     bool     `json:"verbose,omitempty" yaml:"verbose,omitempty"`
	Force       bool     `json:"force,omitempty" yaml:"force,omitempty"`
	Restart     bool     `json:"restart,omitempty" yaml:"restart,omitempty"`
	KubeContext string   `json:"kubeContext,omitempty" yaml:"kubeContext,omitempty"`
	BuildArgs   []string `json:"buildArgs,omitempty" yaml:"buildArgs,omitempty"`
	Platform    string   `json:"platform,omitempty" yaml:"platform,omitempty"`
	Timeout     string   `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	Progress    string   `json:"progress,omitempty" yaml:"progress,omitempty"`
}

var buildArgs BuildArgs

var buildCmd = &cobra.Command{
	Use: "kb",
	RunE: func(cmd *cobra.Command, args []string) error {
		log := logger.NewZapLoggerFromEnv()
		module, err := kb.NewProcess(
			buildArgs.Concurrency,
			log,
		).GetModule(buildArgs.File)
		if err != nil {
			return err
		}
		start := time.Now()
		if err := module.Build(&kb.BuildOptions{
			NoCache:    buildArgs.NoCache,
			Squash:     buildArgs.Squash,
			Tag:        buildArgs.Tag,
			Builder:    buildArgs.Builder,
			Repository: buildArgs.Repository,
			NoPush:     buildArgs.NoPush,
			NoPushDeps: buildArgs.NoPushDeps,
			SkipHooks:  buildArgs.SkipHooks,
			Verbose:    buildArgs.Verbose,
			Force:      buildArgs.Force,
			BuildArgs:  buildArgs.BuildArgs,
			Context:    buildArgs.KubeContext,
			Platform:   buildArgs.Platform,
			Timeout:    buildArgs.Timeout,
			Progress:   buildArgs.Progress,
		}); err != nil {
			return err
		}
		log.Info("Build successful",
			zap.String("elapsed", time.Since(start).String()))
		if buildArgs.Restart {
			start := time.Now()
			log.Info("Restarting containers...", zap.String("kubeContext", buildArgs.KubeContext))
			if err := module.RestartContainers(
				module.BuiltImages,
				buildArgs.Verbose,
				buildArgs.KubeContext,
			); err != nil {
				return err
			}
			log.Info("Containers restarted",
				zap.String("elapsed", time.Since(start).String()))
		}
		return nil
	},
}

func init() {
	buildCmd.PersistentFlags().StringVarP(&buildArgs.File, "file", "f", "./kb.yaml", "Path to kb.yaml file")
	buildCmd.PersistentFlags().IntVarP(&buildArgs.Concurrency, "concurrency", "c", runtime.NumCPU(), "number of parallel build jobs")
	buildCmd.PersistentFlags().BoolVar(&buildArgs.NoCache, "no-cache", false, "build images from scratch")
	buildCmd.PersistentFlags().StringVarP(&buildArgs.Tag, "tag", "t", "", "docker image tag")
	buildCmd.PersistentFlags().BoolVar(&buildArgs.Squash, "squash", false, "squashes newly built layers into a single new layer (docker experimental feature)")
	buildCmd.PersistentFlags().StringVar(&buildArgs.Builder, "builder", "docker", "builder backend (docker or kaniko)")
	buildCmd.PersistentFlags().BoolVar(&buildArgs.NoPush, "no-push", false, "do not push built images")
	buildCmd.PersistentFlags().BoolVar(&buildArgs.NoPushDeps, "no-push-deps", false, "do not push dependencies")
	buildCmd.PersistentFlags().StringVar(&buildArgs.Repository, "repository", "", "push repository override (e.g. localhost:5000)")
	buildCmd.PersistentFlags().BoolVar(&buildArgs.SkipHooks, "skip-hooks", false, "skip before: and after: hooks")
	buildCmd.PersistentFlags().BoolVarP(&buildArgs.Verbose, "verbose", "v", false, "verbose output (pipe build messages to stdout)")
	buildCmd.PersistentFlags().BoolVar(&buildArgs.Force, "force", false, "build regardless of digest")
	buildCmd.PersistentFlags().BoolVarP(&buildArgs.Restart, "restart", "r", false, "restart pods with out-of-date images after build")
	buildCmd.PersistentFlags().StringVar(&buildArgs.KubeContext, "kube-context", "", "kubectl context (uses current by default)")
	buildCmd.PersistentFlags().StringSliceVar(&buildArgs.BuildArgs, "build-arg", nil, "docker build arguments")
	buildCmd.PersistentFlags().StringVar(&buildArgs.Platform, "platform", "", "build platform override (e.g. linux/amd64,linux/arm64, comma-delimit for multiple)")
	buildCmd.PersistentFlags().StringVar(&buildArgs.Timeout, "timeout", "", "build timeout (e.g. 10m, 1h, 1h30m) some cross-platform builds will hang without this")
	buildCmd.PersistentFlags().StringVar(&buildArgs.Progress, "progress", "", "build progress [auto|plain|tty] (see docker buildx build --help)")
}

// ConfigureCommand a function for adding a command to the application
func ConfigureCommand(cmd *cobra.Command) {
	buildCmd.AddCommand(cmd)
}
