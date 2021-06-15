package main

import (
	"runtime"
	"time"

	"github.com/midcontinentcontrols/kindest/pkg/kindest"
	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

type TestArgs struct {
	BuildArgs

	KubeContext string `json:"kubeContext,omitempty" yaml:"kubeContext,omitempty"`
	Kind        string `json:"kind,omitempty" yaml:"kind,omitempty"`
}

var testArgs TestArgs

var testCmd = &cobra.Command{
	Use: "test",
	RunE: func(cmd *cobra.Command, args []string) error {
		log := logger.NewZapLoggerFromEnv()
		p := kindest.NewProcess(
			testArgs.Concurrency,
			log,
		)
		module, err := p.GetModule(testArgs.File)
		if err != nil {
			return err
		}
		start := time.Now()
		if err := module.RunTests(&kindest.TestOptions{
			BuildOptions: kindest.BuildOptions{
				NoCache:    testArgs.NoCache,
				Squash:     testArgs.Squash,
				Tag:        testArgs.Tag,
				Builder:    testArgs.Builder,
				Repository: testArgs.Repository,
				NoPush:     testArgs.NoPush,
				SkipHooks:  testArgs.SkipHooks,
				Verbose:    testArgs.Verbose,
				Force:      testArgs.Force,
			},
			KubeContext: testArgs.KubeContext,
			Kind:        testArgs.Kind,
		}, p, log); err != nil {
			return err
		}
		log.Info("Build successful", zap.String("elapsed", time.Since(start).String()))
		return nil
	},
}

func init() {
	ConfigureCommand(testCmd)
	testCmd.PersistentFlags().StringVarP(&testArgs.File, "file", "f", "./kindest.yaml", "Path to kindest.yaml file")
	testCmd.PersistentFlags().IntVarP(&testArgs.Concurrency, "concurrency", "c", runtime.NumCPU(), "number of parallel build jobs (defaults to num cpus)")
	testCmd.PersistentFlags().BoolVar(&testArgs.NoCache, "no-cache", false, "build images from scratch")
	testCmd.PersistentFlags().StringVarP(&testArgs.Tag, "tag", "t", "latest", "docker image tag")
	testCmd.PersistentFlags().BoolVar(&testArgs.Squash, "squash", false, "squashes newly built layers into a single new layer (docker experimental feature)")
	testCmd.PersistentFlags().StringVar(&testArgs.Builder, "builder", "docker", "builder backend (docker or kaniko)")
	testCmd.PersistentFlags().BoolVar(&testArgs.NoPush, "no-push", false, "do not push built images")
	testCmd.PersistentFlags().StringVar(&testArgs.Repository, "repository", "", "push repository override (e.g. localhost:5000)")
	testCmd.PersistentFlags().BoolVar(&testArgs.SkipHooks, "skip-hooks", false, "skip before: and after: hooks")
	testCmd.PersistentFlags().BoolVarP(&testArgs.Verbose, "verbose", "v", false, "verbose output (pipe build messages to stdout)")
	testCmd.PersistentFlags().BoolVar(&testArgs.Force, "force", false, "build regardless of digest")

	testCmd.PersistentFlags().StringVar(&testArgs.KubeContext, "kube-context", "", "kubectl context (uses current by default)")
	testCmd.PersistentFlags().StringVar(&testArgs.Kind, "kind", "", "Kubernetes-IN-Docker cluster name, for local testing")
}
