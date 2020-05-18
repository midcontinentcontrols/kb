package kindest

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/google/uuid"
	"github.com/jhoonb/archivex"
	"github.com/prometheus/common/log"
	"go.uber.org/zap"
)

type streamMsgT struct {
	Stream string `json:"stream"`
}

func Build(spec *kindestSpec, rootPath string) error {
	if err := spec.Validate(rootPath); err != nil {
		return err
	}
	cli, err := client.NewEnvClient()
	if err != nil {
		return err
	}
	log.Info("Building", zap.String("rootPath", rootPath))
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
			CacheFrom:  []string{spec.Name + ":latest"},
			Dockerfile: spec.Build.Docker.Dockerfile,
			BuildArgs:  buildArgs,
		},
	)
	if err != nil {
		return err
	}
	rd := bufio.NewReader(resp.Body)
	for {
		message, err := rd.ReadString('\n')
		if err != nil {
			break
		}
		var streamMsg streamMsgT
		if err := json.Unmarshal([]byte(message), &streamMsg); err != nil {
			return fmt.Errorf("failed to unmarshal docker message '%v': %v", message, err)
		}
		log.Info("Docker", zap.String("message", streamMsg.Stream))
	}
	log.Info("Successfully built image",
		zap.String("resp.OSType", resp.OSType))
	//cli.ImageTag(
	//	context.TODO(),
	//	"imageID",
	//	"ref",
	//)
	return nil
}
