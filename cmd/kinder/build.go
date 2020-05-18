package main

import (
	"io/ioutil"
	"os"
	"path/filepath"

	yaml "sigs.k8s.io/yaml"

	"github.com/midcontinentcontrols/kinder/pkg/kinder"
	"github.com/spf13/cobra"
)

type BuildArgs struct {
	File string `json:"file,omitempty"`
}

var buildArgs BuildArgs

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		var file string
		if buildArgs.File != "" {
			file = buildArgs.File
		} else {
			file = filepath.Join(dir, "kinder.yaml")
		}
		docBytes, err := ioutil.ReadFile(file)
		if err != nil {
			return err
		}
		spec := &kinder.KinderSpec{}
		if err := yaml.Unmarshal(docBytes, spec); err != nil {
			return err
		}
		return kinder.Build(spec, dir)
	},
}

func init() {
	ConfigureCommand(buildCmd)
	buildCmd.PersistentFlags().StringVarP(&buildArgs.File, "file", "f", "./kinder.yaml", "Path to kinder.yaml file")
}
