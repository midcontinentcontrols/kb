package kindest

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/strslice"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/docker/docker/api/types"
	containertypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/term"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func waitForContainer(containerName string, cli client.APIClient) error {
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

func (b *BuildSpec) buildKanikoLocal(
	manifestPath string,
	options *BuildOptions,
) error {
	cli, err := client.NewEnvClient()
	if err != nil {
		return err
	}
	containerName := "kaniko-" + uuid.New().String()[:8]
	kanikoImage := "gcr.io/kaniko-project/executor:latest"
	resp, err := cli.ImagePull(
		context.TODO(),
		kanikoImage,
		types.ImagePullOptions{
			RegistryAuth: "asdf",
		},
	)
	if err != nil {
		return err
	}
	termFd, isTerm := term.GetFdInfo(os.Stderr)
	if err := jsonmessage.DisplayJSONMessagesStream(
		resp,
		os.Stderr,
		termFd,
		isTerm,
		nil,
	); err != nil {
		return err
	}
	info, err := cli.ContainerCreate(
		context.TODO(),
		&containertypes.Config{
			Image: kanikoImage,
			Cmd: strslice.StrSlice([]string{
				"--dockerfile=Dockerfile",
				"--context=tar://stdin",
				"--destination=docker.io/midcontinentcontrols/kind-test",
			}),
		},
		&containertypes.HostConfig{
			Mounts: []mount.Mount{{
				Type:   mount.TypeBind,
				Source: "docker-config",
				Target: "/kaniko/.docker",
			}},
			RestartPolicy: containertypes.RestartPolicy{
				Name: "no",
			},
		},
		nil,
		containerName,
	)
	if err != nil {
		return err
	}
	if err := waitForContainer(containerName, cli); err != nil {
		return err
	}
	log := log.With(zap.String("id", info.ID))
	log.Info("Created container")
	for _, warning := range info.Warnings {
		log.Warn("Container create warning", zap.String("message", warning))
	}
	return fmt.Errorf("unimplemented")
}

func (b *BuildSpec) buildKanikoRemote(
	context string,
	manifestPath string,
	options *BuildOptions,
) error {
	return fmt.Errorf("unimplemented")
}

func kanikoPod(b *BuildSpec) (*corev1.Pod, error) {
	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	dockerconfigjson, err := ioutil.ReadFile(filepath.Join(u.HomeDir, ".docker", "config.json"))
	if err != nil {
		return nil, err
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kaniko-" + uuid.New().String()[:8],
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:            "kaniko",
				Image:           "gcr.io/kaniko-project/executor:debug",
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command: []string{
					"/busybox/sh",
					"-c",
					`set -e;
					echo "$dockerconfigjson" > /kaniko/.docker/config.json;
					echo "Tailing null...";
					tail -f /dev/null`,
				},
				Env: []corev1.EnvVar{{
					Name:  "dockerconfigjson",
					Value: string(dockerconfigjson),
				}},
			}},
		},
	}, nil
}

func waitForPod(name, namespace string, client *kubernetes.Clientset) error {
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

func (b *BuildSpec) buildKaniko(
	manifestPath string,
	options *BuildOptions,
) (err error) {
	client, config, err := clientForContext(options.Context)
	if err != nil {
		return err
	}
	pods := client.CoreV1().Pods("default")
	// TODO: configure push secret
	// mount $HOME/.docker/config.json into /kaniko/.docker/config.json
	pod, err := kanikoPod(b)
	if err != nil {
		return err
	}
	if pod, err = pods.Create(
		context.TODO(),
		pod,
		metav1.CreateOptions{},
	); err != nil {
		return err
	}
	defer func() {
		if err2 := pods.Delete(
			context.TODO(),
			pod.Name,
			metav1.DeleteOptions{},
		); err2 != nil {
			if err == nil {
				err = err2
			} else {
				log.Error("failed to delete pod", zap.String("err", err2.Error()))
			}
		}
	}()
	resolvedDockerfile, err := resolveDockerfile(
		manifestPath,
		b.Dockerfile,
		b.Context,
	)
	if err != nil {
		return err
	}
	tarPath, err := b.tarBuildContext(manifestPath, options)
	if err != nil {
		return err
	}
	defer os.Remove(tarPath)
	if err := waitForPod(pod.Name, pod.Namespace, client); err != nil {
		return err
	}
	tarData, err := ioutil.ReadFile(tarPath)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if n, err := zw.Write(tarData); err != nil {
		return err
	} else if n != len(tarData) {
		return fmt.Errorf("wrong num bytes")
	}
	if err := zw.Close(); err != nil {
		return err
	}
	command := []string{
		"/kaniko/executor",
		"--dockerfile=" + resolvedDockerfile,
		"--context=tar://stdin",
	}
	if options.NoPush {
		command = append(command, "--no-push")
	} else {
		command = append(command, "--destination="+b.Name)
	}
	if err := execInPod(
		client,
		config,
		pod,
		&corev1.PodExecOptions{
			Command: command,
			Stdin:   true,
			Stdout:  true,
			Stderr:  true,
			TTY:     false,
		},
		bytes.NewReader(buf.Bytes()),
		os.Stdout,
		os.Stderr,
	); err != nil {
		return err
	}
	return nil
}

func execCommand(command string, args ...interface{}) error {
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
	defer NewWaitingMessage(interpolated, 5*time.Second).Stop()
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
