package main

import (
	"runtime"
	"time"

	"github.com/midcontinentcontrols/kindest/pkg/kindest"
	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var test2Cmd = &cobra.Command{
	Use: "test2",
	RunE: func(cmd *cobra.Command, args []string) error {
		kubeContext := testArgs.KubeContext
		start := time.Now()
		log := logger.NewZapLoggerFromEnv()
		p := kindest.NewProcess(testArgs.Concurrency, log)
		module, err := p.GetModule(testArgs.File)
		if err != nil {
			return err
		}
		if err := module.RunTests2(
			&kindest.TestOptions{
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
				KubeContext:   kubeContext,
				Kind:          testArgs.Kind,
				Transient:     testArgs.Transient,
				Namespace:     testArgs.Namespace,
				SkipBuild:     testArgs.SkipBuild,
				SkipTestBuild: testArgs.SkipTestBuild,
				SkipDeploy:    testArgs.SkipDeploy,
			},
			p,
			log,
		); err != nil {
			return err
		}
		log.Info("Test completed", zap.String("elapsed", time.Since(start).String()))
		return nil
	},
}

func init() {
	ConfigureCommand(test2Cmd)
	test2Cmd.PersistentFlags().StringVarP(&testArgs.File, "file", "f", "./kindest.yaml", "Path to kindest.yaml file")
	test2Cmd.PersistentFlags().IntVarP(&testArgs.Concurrency, "concurrency", "c", runtime.NumCPU(), "number of parallel build jobs (defaults to num cpus)")
	test2Cmd.PersistentFlags().BoolVar(&testArgs.NoCache, "no-cache", false, "build images from scratch")
	test2Cmd.PersistentFlags().StringVarP(&testArgs.Tag, "tag", "t", "latest", "docker image tag")
	test2Cmd.PersistentFlags().BoolVar(&testArgs.Squash, "squash", false, "squashes newly built layers into a single new layer (docker experimental feature)")
	test2Cmd.PersistentFlags().StringVar(&testArgs.Builder, "builder", "docker", "builder backend (docker or kaniko)")
	test2Cmd.PersistentFlags().BoolVar(&testArgs.NoPush, "no-push", false, "do not push built images")
	test2Cmd.PersistentFlags().StringVar(&testArgs.Repository, "repository", "", "push repository override (e.g. localhost:5000)")
	test2Cmd.PersistentFlags().BoolVar(&testArgs.SkipHooks, "skip-hooks", false, "skip before: and after: hooks")
	test2Cmd.PersistentFlags().BoolVarP(&testArgs.Verbose, "verbose", "v", false, "verbose output (pipe build messages to stdout)")
	test2Cmd.PersistentFlags().BoolVar(&testArgs.Force, "force", false, "build regardless of digest")

	test2Cmd.PersistentFlags().StringVarP(&testArgs.Namespace, "namespace", "n", "default", "test pod namespace")
	test2Cmd.PersistentFlags().StringVar(&testArgs.KubeContext, "kube-context", "", "kubectl context (uses current by default)")
	test2Cmd.PersistentFlags().StringVar(&testArgs.Kind, "kind", "", "Kubernetes-IN-Docker cluster name, for local testing")
	test2Cmd.PersistentFlags().BoolVar(&testArgs.Transient, "transient", false, "Delete kind cluster on exit")
	test2Cmd.PersistentFlags().BoolVar(&testArgs.SkipBuild, "skip-build", false, "Skip automatic build")
	test2Cmd.PersistentFlags().BoolVar(&testArgs.SkipTestBuild, "skip-test-build", false, "Skip building test images")
	test2Cmd.PersistentFlags().BoolVar(&testArgs.SkipDeploy, "skip-deploy", false, "Skip automatic deploy")
}
