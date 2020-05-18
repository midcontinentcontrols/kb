package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

type BuildArgs struct {
}

var buildArgs TestArgs

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("unimplemented")
	},
}

func init() {
	ConfigureCommand(testCmd)
}
