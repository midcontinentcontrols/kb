package main

import (
	"github.com/spf13/cobra"
)

// ConfigureCommand a function for adding a command to the application
func ConfigureCommand(cmd *cobra.Command) {
	rootCmd.AddCommand(cmd)
}

var rootCmd = &cobra.Command{
	Use: "kindest",
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}
