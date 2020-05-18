package kinder

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/google/uuid"
	"github.com/jhoonb/archivex"
	"go.uber.org/zap"
)

func Build(spec *KinderSpec, rootPath string) error {
	if err := spec.Validate(rootPath); err != nil {
		return err
	}
	cli, err := client.NewEnvClient()
	if err != nil {
		return err
	}
	log.Info("Building", zap.String("rootPath", rootPath))
	ctxPath := fmt.Sprintf("build-%s.tar", uuid.New().String())
	tar := new(archivex.TarFile)
	tar.Create(ctxPath)
	tar.AddAll(rootPath, false)
	tar.Close()
	//defer os.Remove(ctxPath)
	dockerBuildContext, err := os.Open(ctxPath)
	if err != nil {
		return err
	}
	defer dockerBuildContext.Close()
	var dockerfilePath string
	if spec.Build.Docker.Dockerfile != "" {
		dockerfilePath = filepath.Join(rootPath, spec.Build.Docker.Dockerfile)
	} else {
		dockerfilePath = filepath.Join(rootPath, "Dockerfile")
	}
	dockerfilePath = "./Dockerfile"
	buildArgs := make(map[string]*string)
	for _, arg := range spec.Build.Docker.BuildArgs {
		buildArgs[arg.Name] = &arg.Value
	}
	resp, err := cli.ImageBuild(
		context.TODO(),
		nil,
		types.ImageBuildOptions{
			Dockerfile: dockerfilePath,
			Context:    dockerBuildContext,
			BuildArgs:  buildArgs,
		},
	)
	if err != nil {
		return err
	}
	io.Copy(os.Stdout, resp.Body)
	log.Info("Successfully built image")
	return nil
}
