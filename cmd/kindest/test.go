package main

import (
	"runtime"

	"github.com/spf13/cobra"
)

type TestArgs struct {
	File        string `json:"file,omitempty" yaml:"file,omitempty"`
	Concurrency int    `json:"concurrency,omitempty" yaml:"concurrency,omitempty"`
	NoCache     bool   `json:"nocache,omitempty" yaml:"nocache,omitempty"`
	Squash      bool   `json:"squash,omitempty" yaml:"squash,omitempty"`
	Tag         string `json:"tag,omitempty" yaml:"tag,omitempty"`
	Builder     string `json:"builder,omitempty" yaml:"builder,omitempty"`
	Repository  string `json:"repository,omitempty" yaml:"repository,omitempty"`
	NoPush      bool   `json:"noPush,omitempty" yaml:"noPush,omitempty"`
	SkipHooks   bool   `json:"skipHooks,omitempty" yaml:"skipHooks,omitempty"`
	KubeContext string `json:"kubeContext,omitempty" yaml:"kubeContext,omitempty"`
	Kind        string `json:"kind,omitempty" yaml:"kind,omitempty"`
}

var testArgs TestArgs

var testCmd = &cobra.Command{
	Use: "test",
	RunE: func(cmd *cobra.Command, args []string) error {
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
	testCmd.PersistentFlags().StringVar(&testArgs.KubeContext, "kube-context", "", "kubectl context (uses current by default)")
	testCmd.PersistentFlags().StringVar(&testArgs.Kind, "kind", "", "Kubernetes-IN-Docker cluster name, for local testing")
}
