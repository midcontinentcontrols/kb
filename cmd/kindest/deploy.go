package main

import (
	"time"

	"github.com/midcontinentcontrols/kindest/pkg/kindest"
	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

type DeployArgs struct {
	File        string `json:"file,omitempty" yaml:"file,omitempty"`
	KubeContext string `json:"kubeContext,omitempty" yaml:"kubeContext,omitempty"`
}

var deployArgs DeployArgs

var deployCmd = &cobra.Command{
	Use: "deploy",
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
		if err := module.Deploy(&kindest.DeployOptions{
			KubeContext: deployArgs.KubeContext,
		}); err != nil {
			return err
		}
		log.Info("Build successful", zap.String("elapsed", time.Since(start).String()))
		return nil
	},
}

func init() {
	ConfigureCommand(deployCmd)
	deployCmd.PersistentFlags().StringVarP(&deployArgs.File, "file", "f", "./kindest.yaml", "Path to kindest.yaml file")
	deployCmd.PersistentFlags().StringVar(&deployArgs.KubeContext, "kube-context", "", "kubectl context override")
}
