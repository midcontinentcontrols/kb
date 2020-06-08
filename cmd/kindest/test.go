package main

import (
	"runtime"

	"github.com/midcontinentcontrols/kindest/pkg/kindest"

	"github.com/spf13/cobra"
)

var testOptions kindest.TestOptions

var testCmd = &cobra.Command{
	Use: "test",
	RunE: func(cmd *cobra.Command, args []string) error {
		return kindest.Test(&testOptions)
	},
}

func init() {
	ConfigureCommand(testCmd)
	testCmd.PersistentFlags().StringVarP(&testOptions.File, "file", "f", "./kindest.yaml", "Path to kindest.yaml file")
	testCmd.PersistentFlags().IntVarP(&testOptions.Concurrency, "concurrency", "c", runtime.NumCPU(), "number of parallel build jobs (defaults to num cpus)")
	testCmd.PersistentFlags().StringVar(&testOptions.Context, "context", "", "kubecontext (on-cluster build)")
	testCmd.PersistentFlags().StringVar(&testOptions.Kind, "kind", "", "copy image to kind cluster instead of pushing")
	testCmd.PersistentFlags().BoolVar(&testOptions.NoRegistry, "no-registry", false, "disable local registry")
	testCmd.PersistentFlags().BoolVar(&testOptions.SkipBuild, "skip-build", false, "skip pre-test building")
	testCmd.PersistentFlags().StringVar(&testOptions.Builder, "builder", "docker", "builder backend (docker or kaniko)")
}
