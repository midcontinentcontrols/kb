package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/transport/spdy"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
)

// PortForwardOptions contains all the options for running the port-forward cli command.
type PortForwardOptions struct {
	Namespace     string
	PodName       string
	RESTClient    *restclient.RESTClient
	Config        *restclient.Config
	Client        *kubernetes.Clientset
	Ports         []string
	PortForwarder portForwarder
	StopChannel   chan struct{}
	ReadyChannel  chan struct{}
}

func NewCmdPortForward(client *kubernetes.Clientset, cmdOut, cmdErr io.Writer) *cobra.Command {
	opts := &PortForwardOptions{
		Client: client,
		PortForwarder: &defaultPortForwarder{
			cmdOut: cmdOut,
			cmdErr: cmdErr,
		},
	}
	cmd := &cobra.Command{
		Use:   "port-forward POD [LOCAL_PORT:]REMOTE_PORT [...[LOCAL_PORT_N:]REMOTE_PORT_N]",
		Short: "Forward one or more local ports to a pod",
		Long:  "Forward one or more local ports to a pod.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := opts.Complete(cmd, args, cmdOut, cmdErr); err != nil {
				return err
			}
			if err := opts.Validate(); err != nil {
				return err
			}
			if err := opts.RunPortForward(); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringP("pod", "p", "", "Pod name")
	return cmd
}

type portForwarder interface {
	ForwardPorts(method string, url *url.URL, opts PortForwardOptions) error
}

type defaultPortForwarder struct {
	cmdOut, cmdErr io.Writer
}

func (f *defaultPortForwarder) ForwardPorts(method string, url *url.URL, opts PortForwardOptions) error {
	// TODO: inject config
	cfg := &restclient.Config{}
	transport, upgrader, err := spdy.RoundTripperFor(cfg)
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", url)
	fw, err := portforward.New(dialer, opts.Ports, opts.StopChannel, opts.ReadyChannel, f.cmdOut, f.cmdErr)
	if err != nil {
		return err
	}
	return fw.ForwardPorts()
}

// Complete completes all the required options for port-forward cmd.
func (o *PortForwardOptions) Complete(cmd *cobra.Command, args []string, cmdOut io.Writer, cmdErr io.Writer) error {
	o.PodName = ""
	if len(o.PodName) == 0 && len(args) == 0 {
		return fmt.Errorf("POD is required for port-forward")
	}
	if len(o.PodName) != 0 {
		o.Ports = args
	} else {
		o.PodName = args[0]
		o.Ports = args[1:]
	}
	o.Namespace = ""
	o.StopChannel = make(chan struct{}, 1)
	o.ReadyChannel = make(chan struct{})
	return nil
}

// Validate validates all the required options for port-forward cmd.
func (o PortForwardOptions) Validate() error {
	if len(o.PodName) == 0 {
		return fmt.Errorf("pod name must be specified")
	}

	if len(o.Ports) < 1 {
		return fmt.Errorf("at least 1 PORT is required for port-forward")
	}

	if o.PortForwarder == nil || o.RESTClient == nil || o.Config == nil {
		return fmt.Errorf("client, client config, restClient, and portforwarder must be provided")
	}
	return nil
}

// RunPortForward implements all the necessary functionality for port-forward cmd.
func (o PortForwardOptions) RunPortForward() error {
	pod, err := o.Client.CoreV1().Pods(o.Namespace).Get(context.TODO(), o.PodName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	if pod.Status.Phase != corev1.PodRunning {
		return fmt.Errorf("unable to forward port because pod is not running. Current status=%v", pod.Status.Phase)
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	defer signal.Stop(signals)

	go func() {
		<-signals
		if o.StopChannel != nil {
			close(o.StopChannel)
		}
	}()

	req := o.RESTClient.Post().
		Resource("pods").
		Namespace(o.Namespace).
		Name(pod.Name).
		SubResource("portforward")

	return o.PortForwarder.ForwardPorts("POST", req.URL(), o)
}
