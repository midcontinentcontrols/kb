package kindest

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"go.uber.org/zap"

	"github.com/midcontinentcontrols/kindest/pkg/logger"
)

// buildxDocker is a copy of buildDocker with the ImageBuild call replaced with
// a call to ImageBuildX. This is a temporary
func buildxDocker(
	ctx context.Context,
	spec *BuildSpec,
	dest string,
	tag string,
	buildContext []byte,
	relativeDockerfile string,
	options *BuildOptions,
	log logger.Logger,
) error {
	repo := options.Repository
	if repo != "" && !strings.HasSuffix(repo, "/") {
		repo += "/"
	}
	args := []string{
		"buildx", "build",
		"-t", dest,
		"-f", relativeDockerfile,
		"--build-arg", "KINDEST_REPOSITORY=" + repo,
		"--build-arg", "KINDEST_TAG=" + tag,
	}
	for _, arg := range spec.BuildArgs {
		args = append(args, "--build-arg", arg.Name+"="+arg.Value)
	}
	if len(options.BuildArgs) > 0 {
		for _, arg := range options.BuildArgs {
			args = append(args, "--build-arg", arg)
			log.Debug("build arg", zap.String("arg", arg))
		}
	}
	if options.Platform != "" {
		args = append(args, "--platform="+options.Platform)
	}
	if !options.NoPush {
		args = append(args, "--push")
	}
	args = append(args, "-") // context from stdin
	cmd := exec.CommandContext(
		ctx,
		"docker",
		args...,
	)
	cmd.Dir = spec.Context
	cmd.Stdin = bytes.NewReader(buildContext)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Debug("Executing docker buildx command", zap.Strings("args", args))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker buildx build: %v", err)
	}
	if options.NoPush {
		log.Debug("Skipping push",
			zap.Bool("options.noPush", options.NoPush),
			zap.Bool("spec.SkipPush", spec.SkipPush))
	}
	return nil
}
