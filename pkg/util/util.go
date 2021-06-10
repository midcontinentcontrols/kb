package util

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/midcontinentcontrols/kindest/pkg/logger"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"go.uber.org/zap"
)

func EnsureImagePulled(imageName string, cli client.APIClient, log logger.Logger) error {
	resp, err := cli.ImagePull(context.TODO(), imageName, types.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("docker: %v", err)
	}
	body, err := ioutil.ReadAll(resp)
	if err != nil {
		return err
	}
	log.Info(
		"Pulled image",
		zap.String("response", string(body)),
		zap.String("imageName", imageName))
	return nil
}

func WaitForContainer(
	containerName string,
	cli client.APIClient,
	log logger.Logger,
) error {
	done := make(chan int)
	go func() {
		log := log.With(zap.String("name", containerName))
		for {
			select {
			case <-time.After(5 * time.Second):
				log.Info("Still waiting for container")
			case <-done:
				return
			}
		}
	}()
	defer func() {
		done <- 0
		close(done)
	}()
	for {
		info, err := cli.ContainerInspect(context.TODO(), containerName)
		if err != nil {
			return err
		}
		if info.State == nil {
			panic("nil container state")
		}
		switch info.State.Status {
		case "created":
			if err := cli.ContainerStart(
				context.TODO(),
				containerName,
				types.ContainerStartOptions{},
			); err != nil {
				return fmt.Errorf("error starting container: %v", err)
			}
			time.Sleep(time.Second)
		case "running":
			return nil
		default:
			return fmt.Errorf("unexpected container state '%s'", info.State.Status)
		}
	}
}

func WaitForPod(name, namespace string, client *kubernetes.Clientset, log logger.Logger) error {
	timeout := time.Second * 120
	delay := time.Second
	start := time.Now()
	pods := client.CoreV1().Pods(namespace)
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); {
		pod, err := pods.Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		switch pod.Status.Phase {
		case corev1.PodPending:
			log.Info("Waiting on pod",
				zap.String("elapsed", time.Now().Sub(start).String()),
				zap.String("timeout", timeout.String()))
			time.Sleep(delay)
			continue
		case corev1.PodRunning:
			return nil
		default:
			return fmt.Errorf("unexpected pod phase '%s' for %s.%s", pod.Status.Phase, pod.Name, pod.Namespace)
		}
	}
	return fmt.Errorf("pod failed to be Ready within %s", timeout.String())
}

func SanitizeImageName(host, image, tag string) string {
	if tag == "" {
		tag = "latest"
	}
	n := len(host)
	if n == 0 {
		return fmt.Sprintf("%s:%s", image, tag)
	}
	if host[n-1] == '/' {
		host = host[:n-1]
	}
	return fmt.Sprintf("%s/%s:%s", host, image, tag)
}

func ExecCommand(log logger.Logger, command string, args ...interface{}) error {
	var interpolated string
	if len(args) > 0 {
		interpolated = fmt.Sprintf(command, args...)
	} else {
		interpolated = command
	}
	cmd := exec.Command("bash", "-c", interpolated)
	log.Info("Running command", zap.String("cmd", interpolated))
	r, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	resultStdout := make(chan interface{}, 1)
	resultStderr := make(chan interface{}, 1)
	go func() {
		defer close(resultStderr)
		stderr, err := ioutil.ReadAll(r)
		if err != nil && err != io.EOF && err != os.ErrClosed && !strings.Contains(err.Error(), "file already closed") {
			resultStderr <- fmt.Errorf("ReadAll: %v", err)
			return
		}
		resultStderr <- string(stderr)
	}()
	go func() {
		defer close(resultStdout)
		stdout, err := ioutil.ReadAll(stdout)
		if err != nil && err != io.EOF && err != os.ErrClosed && !strings.Contains(err.Error(), "file already closed") {
			resultStdout <- fmt.Errorf("ReadAll: %v", err)
			return
		}
		resultStdout <- string(stdout)
	}()
	//defer NewWaitingMessage(interpolated, 5*time.Second, log).Stop()
	if err := cmd.Run(); err != nil && err != io.EOF && err != os.ErrClosed {
		stdoutStr, _ := (<-resultStdout).(string)
		vStderr := <-resultStderr
		if stderr, ok := vStderr.(string); ok {
			return fmt.Errorf("%v: stdout: %s\nstderr: %s", err, stdoutStr, stderr)
		} else if err2, ok := vStderr.(error); ok {
			return fmt.Errorf("%v: stdout: %s\nstderr: %s", err, err2, stdoutStr)
		} else {
			panic("unreachable branch detected")
		}
	}
	return nil
}
