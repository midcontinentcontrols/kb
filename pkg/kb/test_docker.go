package kb

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/docker/docker/api/types/mount"

	"github.com/midcontinentcontrols/kb/pkg/logger"

	"github.com/docker/docker/api/types/strslice"

	"github.com/docker/docker/api/types"
	containertypes "github.com/docker/docker/api/types/container"

	"github.com/docker/docker/client"
	"go.uber.org/zap"
)

func (t *TestSpec) runDocker(rootPath string, log logger.Logger) error {
	cli, err := client.NewClientWithOpts()
	if err != nil {
		return err
	}
	var env []string
	for _, v := range t.Variables {
		env = append(env, v.Name+"="+v.Value)
	}
	var mounts []mount.Mount
	for _, volume := range t.Env.Docker.Volumes {
		source := volume.Source
		if !filepath.IsAbs(source) {
			source = filepath.Clean(filepath.Join(rootPath, source))
			source, err = filepath.Abs(source)
			if err != nil {
				return err
			}
		}
		ty := volume.Type
		if ty == "" {
			ty = "bind"
		}
		mounts = append(mounts, mount.Mount{
			Type:        mount.Type(ty),
			Source:      source,
			Target:      volume.Target,
			ReadOnly:    volume.ReadOnly,
			Consistency: mount.Consistency(volume.Consistency),
		})
	}
	var resp containertypes.ContainerCreateCreatedBody
	resp, err = cli.ContainerCreate(
		context.TODO(),
		&containertypes.Config{
			Image: t.Build.Name + ":latest",
			Env:   env,
			Cmd:   strslice.StrSlice(t.Build.Command),
		},
		&containertypes.HostConfig{
			Mounts: mounts,
		},
		nil,
		nil,
		"",
	)
	if err != nil {
		return fmt.Errorf("error creating container: %v", err)
	}
	container := resp.ID
	log = log.With(zap.String("test.Name", t.Name))
	defer func() {
		if rmerr := cli.ContainerRemove(
			context.TODO(),
			container,
			types.ContainerRemoveOptions{},
		); rmerr != nil {
			if err == nil {
				err = rmerr
			} else {
				log.Error("error removing container",
					zap.String("id", container),
					zap.String("message", rmerr.Error()))
			}
		}
	}()
	for _, warning := range resp.Warnings {
		log.Debug("Docker", zap.String("warning", warning))
	}
	if err := cli.ContainerStart(
		context.TODO(),
		container,
		types.ContainerStartOptions{},
	); err != nil {
		return fmt.Errorf("error starting container: %v", err)
	}
	logs, err := cli.ContainerLogs(
		context.TODO(),
		container,
		types.ContainerLogsOptions{
			Follow:     true,
			ShowStdout: true,
			ShowStderr: true,
		},
	)
	if err != nil {
		return fmt.Errorf("error getting logs: %v", err)
	}
	rd := bufio.NewReader(logs)
	for {
		message, err := rd.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		log.Info(message)
	}
	ch, e := cli.ContainerWait(
		context.TODO(),
		container,
		containertypes.WaitConditionNotRunning,
	)
	done := make(chan int, 1)
	go func() {
		start := time.Now()
		for {
			select {
			case <-time.After(3 * time.Second):
				log.Info("Still waiting on container", zap.String("elapsed", time.Since(start).String()))
			case <-done:
				return
			}
		}
	}()
	defer func() {
		done <- 0
		close(done)
	}()
	select {
	case v := <-ch:
		if v.Error != nil {
			return fmt.Errorf("error waiting for container: %v", v.Error.Message)
		}
		if v.StatusCode != 0 {
			return fmt.Errorf("exit code %d", v.StatusCode)
		}
		return nil
	case err := <-e:
		return fmt.Errorf("error waiting for container: %v", err)
	}
}
