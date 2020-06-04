package main

import (
	"github.com/spf13/cobra"
)

type RootOptions struct {
	Dest       string
	Dockerfile string
	Context    string
}

// ConfigureCommand a function for adding a command to the application
func ConfigureCommand(cmd *cobra.Command) {
	rootCmd.AddCommand(cmd)
}

var rootCmd = &cobra.Command{
	Use:   "kindestkaniko",
	Short: "",
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}
