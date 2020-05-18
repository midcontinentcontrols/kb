package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

type TestArgs struct {
}

var testArgs TestArgs

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("unimplemented")
	},
}

func init() {
	ConfigureCommand(testCmd)
}
