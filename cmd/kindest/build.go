package main

import (
	"io/ioutil"
	"os"
	"path/filepath"

	yaml "sigs.k8s.io/yaml"

	"github.com/midcontinentcontrols/kindest/pkg/kindest"
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
			file = filepath.Join(dir, "kindest.yaml")
		}
		docBytes, err := ioutil.ReadFile(file)
		if err != nil {
			return err
		}
		spec := &kindest.kindestSpec{}
		if err := yaml.Unmarshal(docBytes, spec); err != nil {
			return err
		}
		return kindest.Build(spec, dir)
	},
}

func init() {
	ConfigureCommand(buildCmd)
	buildCmd.PersistentFlags().StringVarP(&buildArgs.File, "file", "f", "./kindest.yaml", "Path to kindest.yaml file")
}
