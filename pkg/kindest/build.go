package kindest

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/google/uuid"
	"github.com/jhoonb/archivex"
	"go.uber.org/zap"
)

type streamMsgT struct {
	Stream string `json:"stream"`
}

func Build(
	spec *KindestSpec, // derived from kinder.yaml
	rootPath string, // should contain kinder.yaml
	tag string,
	noCache bool,
	squash bool,
) error {
	if err := spec.Validate(rootPath); err != nil {
		return err
	}
	cli, err := client.NewEnvClient()
	if err != nil {
		return err
	}
	log.Info("Building",
		zap.String("rootPath", rootPath),
		zap.String("tag", tag),
		zap.Bool("noCache", noCache))
	ctxPath := fmt.Sprintf("tmp/build-%s.tar", uuid.New().String())
	tar := new(archivex.TarFile)
	tar.Create(ctxPath)
	tar.AddAll(rootPath, false)
	tar.Close()
	defer os.Remove(ctxPath)
	dockerBuildContext, err := os.Open(ctxPath)
	if err != nil {
		return err
	}
	defer dockerBuildContext.Close()
	buildArgs := make(map[string]*string)
	for _, arg := range spec.Build.Docker.BuildArgs {
		buildArgs[arg.Name] = &arg.Value
	}
	resp, err := cli.ImageBuild(
		context.TODO(),
		dockerBuildContext,
		types.ImageBuildOptions{
			NoCache:    noCache,
			CacheFrom:  []string{spec.Name},
			Dockerfile: spec.Build.Docker.Dockerfile,
			BuildArgs:  buildArgs,
			Squash:     squash,
		},
	)
	if err != nil {
		return err
	}
	rd := bufio.NewReader(resp.Body)
	var line string
	for {
		message, err := rd.ReadString('\n')
		if err != nil {
			log.Error("error reading docker build output", zap.String("err", err.Error()))
			break
		}
		var msg struct {
			Stream string `json:"stream"`
		}
		if err := json.Unmarshal([]byte(message), &msg); err != nil {
			return fmt.Errorf("failed to unmarshal docker message '%v': %v", message, err)
		}
		line = msg.Stream
		log.Info("Docker", zap.String("message", line))
	}
	prefix := "Successfully built "
	if !strings.HasPrefix(line, prefix) {
		return fmt.Errorf("expected build success message, got '%s'", line)
	}
	imageID := strings.TrimSpace(line[len(prefix):])
	ref := spec.Name // + ":" + tag
	log.Info("Successfully built image",
		zap.String("imageID", imageID),
		zap.String("ref", ref),
		zap.String("osType", resp.OSType))
	if err := cli.ImageTag(
		context.TODO(),
		imageID,
		ref,
	); err != nil {
		return err
	}
	return nil
}
