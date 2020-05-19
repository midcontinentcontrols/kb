package main

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"go.uber.org/zap"
	yaml "sigs.k8s.io/yaml"

	"github.com/midcontinentcontrols/kindest/pkg/kindest"
	"github.com/spf13/cobra"
)

type BuildArgs struct {
	File    string `json:"file,omitempty"`
	NoCache bool   `json:"nocache,omitempty"`
	Squash  bool   `json:"squash,omitempty"`
	Tag     string `json:"tag,omitempty"`
}

var buildArgs BuildArgs

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "",
	RunE: func(cmd *cobra.Command, args []string) error {
		var dir string
		var file string
		if buildArgs.File != "" {
			file = buildArgs.File
			var err error
			file, err = filepath.Abs(file)
			if err != nil {
				return err
			}
			dir = filepath.Dir(file)
		} else {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			file = filepath.Join(dir, "kindest.yaml")
		}
		docBytes, err := ioutil.ReadFile(file)
		if err != nil {
			return err
		}
		spec := &kindest.KindestSpec{}
		if err := yaml.Unmarshal(docBytes, spec); err != nil {
			return err
		}
		if spec.Build.Docker.Context != "" {
			dir, err = filepath.Abs(filepath.Join(dir, spec.Build.Docker.Context))
			if err != nil {
				return err
			}
			log.Info("Overriding build context", zap.String("path", dir))
		}
		return kindest.Build(
			spec,
			dir,
			buildArgs.Tag,
			buildArgs.NoCache,
			buildArgs.Squash,
		)
	},
}

func init() {
	ConfigureCommand(buildCmd)
	buildCmd.PersistentFlags().StringVarP(&buildArgs.File, "file", "f", "./kindest.yaml", "Path to kindest.yaml file")
	buildCmd.PersistentFlags().StringVarP(&buildArgs.Tag, "tag", "t", "latest", "docker image tag")
	buildCmd.PersistentFlags().BoolVar(&buildArgs.NoCache, "no-cache", false, "build images from scratch")
	buildCmd.PersistentFlags().BoolVar(&buildArgs.Squash, "squash", false, "squashes newly built layers into a single new layer (docker experimental feature)")
}
