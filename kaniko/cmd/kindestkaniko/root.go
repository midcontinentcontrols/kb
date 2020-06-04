package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

type BuildArg struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type RootOptions struct {
	Dest        string
	Dockerfile  string
	Destination string
	Target      string
	BuildArgs   []BuildArg
}

var options RootOptions

// ConfigureCommand a function for adding a command to the application
func ConfigureCommand(cmd *cobra.Command) {
	rootCmd.AddCommand(cmd)
}

var ErrMissingDockerfile = fmt.Errorf("missing dockerfile")

var ErrMissingDestination = fmt.Errorf("missing destination")

func kanikoCommand(options *RootOptions) ([]string, error) {
	if options.Dockerfile == "" {
		return nil, ErrMissingDockerfile
	}
	if options.Destination == "" {
		return nil, ErrMissingDestination
	}
	command := []string{
		"/kaniko/executor",
		"--context=tar://stdin",
		"--dockerfile=" + options.Dockerfile,
		"--destination=" + options.Destination,
	}
	if options.Target != "" {
		command = append(command, "--target="+options.Target)
	}
	for _, buildArg := range options.BuildArgs {
		command = append(command, fmt.Sprintf("--build-arg=%s=%s", buildArg.Name, buildArg.Value))
	}
	return command, nil
}

var rootCmd = &cobra.Command{
	Use: "kindestkaniko",
	RunE: func(cmd *cobra.Command, args []string) error {
		var cmdOut, cmdErr io.Writer
		opts := &PortForwardOptions{
			PortForwarder: &defaultPortForwarder{
				cmdOut: cmdOut,
				cmdErr: cmdErr,
			},
		}
		if err := opts.Complete(cmd, args, cmdOut, cmdErr); err != nil {
			return err
		}
		if err := opts.Validate(); err != nil {
			return err
		}
		if err := opts.RunPortForward(); err != nil {
			return err
		}
		// TODO: open port-forward with kubectl
		//var url *url.URL
		//dialer, err := remotecommand.NewExecutor(portforward.PortForwardOptions{}, method, url)
		//if err != nil {
		//	return err
		//}
		//stopChan := make(chan struct{})
		//readyChan := make(chan struct{})
		//out := bytes.NewBuffer(nil)
		//errOut := bytes.NewBuffer(nil)
		//portforward.New(nil, []string{}, stopChan, readyChan, out, errOut)
		_, err := kanikoCommand(&options)
		if err != nil {
			return err
		}
		return nil
	},
}
