package kindest

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/strslice"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

func kanikoPod(b *BuildSpec) *corev1.Pod {
	// TODO
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kaniko-" + uuid.New().String()[:8],
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:            "kaniko",
				Image:           "gcr.io/kaniko-project/executor:latest",
				ImagePullPolicy: corev1.PullIfNotPresent,
				Args: []string{
					"--dockerfile=Dockerfile",
					"--context=tar://stdin",
					"--destination=gcr.io/my-repo/my-image",
				},
			}},
		},
	}
}

func (b *BuildSpec) buildKaniko(
	manifestPath string,
	options *BuildOptions,
) error {
	client, err := clientForContext(options.Context)
	if err != nil {
		return err
	}
	pods := client.CoreV1().Pods("default")
	pod := kanikoPod(b)
	if pod, err = pods.Create(
		context.TODO(),
		pod,
		metav1.CreateOptions{},
	); err != nil {
		return err
	}
	// TODO: watch kaniko pod
	return nil
}
