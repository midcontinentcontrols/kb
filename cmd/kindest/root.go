package main

import (
	"github.com/spf13/cobra"
)

type RootArgs struct {
}

var rootArgs RootArgs

// ConfigureCommand a function for adding a command to the application
func ConfigureCommand(cmd *cobra.Command) {
	rootCmd.AddCommand(cmd)
}

var rootCmd = &cobra.Command{
	Use:   "kindest",
	Short: "",
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

func init() {
}
