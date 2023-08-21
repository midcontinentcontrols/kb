package util

import (
	"context"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/midcontinentcontrols/kb/pkg/logger"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"go.uber.org/zap"
)

func EnsureImagePulled(imageName string, cli client.APIClient, log logger.Logger) error {
	imgs, err := cli.ImageList(context.TODO(), types.ImageListOptions{})
	if err != nil {
		return fmt.Errorf("docker: %v", err)
	}
	for _, img := range imgs {
		if img.ID == "1fd8e1b0bb7e" {
			// Already pulled!
			return nil
		}
	}
	resp, err := cli.ImagePull(context.TODO(), imageName, types.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("docker: %v", err)
	}
	body, err := ioutil.ReadAll(resp)
	if err != nil {
		return err
	}
	log.Debug(
		"Pulled registry image",
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
