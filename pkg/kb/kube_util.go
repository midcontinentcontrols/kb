package kb

import (
	"fmt"
	"io"
	"net/url"
	"sync"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

// containerToAttach returns a reference to the container to attach to, given
// by name or the first container if name is empty.
func containerToAttachTo(container string, pod *v1.Pod) (*v1.Container, error) {
	if len(container) > 0 {
		for i := range pod.Spec.Containers {
			if pod.Spec.Containers[i].Name == container {
				return &pod.Spec.Containers[i], nil
			}
		}
		for i := range pod.Spec.InitContainers {
			if pod.Spec.InitContainers[i].Name == container {
				return &pod.Spec.InitContainers[i], nil
			}
		}
		return nil, fmt.Errorf("container not found (%s)", container)
	}
	return &pod.Spec.Containers[0], nil
}

// execInPod executes the command in the pod
func execInPod(
	client *kubernetes.Clientset,
	config *restclient.Config,
	pod *v1.Pod,
	options *v1.PodExecOptions,
	stdin io.Reader,
	stdout,
	stderr io.Writer,
) error {
	req := client.CoreV1().RESTClient().Post().Resource("pods").Name(pod.Name).
		Namespace(pod.Namespace).SubResource("exec")
	req.VersionedParams(
		options,
		scheme.ParameterCodec,
	)
	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return err
	}
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		return err
	}
	return nil
}

func startStream(method string, url *url.URL, config *restclient.Config, streamOptions remotecommand.StreamOptions) error {
	exec, err := remotecommand.NewSPDYExecutor(config, method, url)
	if err != nil {
		return err
	}

	return exec.Stream(streamOptions)
}

func getStreamOptions(attachOptions *v1.PodAttachOptions, stdin io.Reader, stdout, stderr io.Writer) remotecommand.StreamOptions {
	var streamOptions remotecommand.StreamOptions
	if attachOptions.Stdin {
		streamOptions.Stdin = stdin
	}

	if attachOptions.Stdout {
		streamOptions.Stdout = stdout
	}

	if attachOptions.Stderr {
		streamOptions.Stderr = stderr
	}

	return streamOptions
}

type stopChan struct {
	c chan struct{}
	sync.Once
}

func newStopChan() *stopChan {
	return &stopChan{c: make(chan struct{})}
}

func (s *stopChan) closeOnce() {
	s.Do(func() {
		close(s.c)
	})
}
