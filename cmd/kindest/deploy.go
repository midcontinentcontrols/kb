package main

import (
	"strings"
	"time"

	"github.com/midcontinentcontrols/kindest/pkg/kindest"
	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

type DeployArgs struct {
	File          string `json:"file,omitempty" yaml:"file,omitempty"`
	Kind          string `json:"kind,omitempty" yaml:"kind,omitempty"`
	KubeContext   string `json:"kubeContext,omitempty" yaml:"kubeContext,omitempty"`
	RestartImages string `json:"restartImages"`
	Wait          bool   `json:"wait"`
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
		if _, err := module.Deploy(&kindest.DeployOptions{
			Kind:          deployArgs.Kind,
			KubeContext:   deployArgs.KubeContext,
			RestartImages: strings.Split(deployArgs.RestartImages, ","),
			Wait:          deployArgs.Wait,
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
	deployCmd.PersistentFlags().StringVar(&deployArgs.Kind, "kind", "", "kind cluster name")
	deployCmd.PersistentFlags().StringVar(&deployArgs.KubeContext, "kube-context", "", "kubectl context override")
	deployCmd.PersistentFlags().StringVar(&deployArgs.RestartImages, "restart-images", "", "comma-separated list of images used to restart deployments")
	deployCmd.PersistentFlags().BoolVarP(&deployArgs.Wait, "wait", "w", false, "wait for successful deployment")
}
