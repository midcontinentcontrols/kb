package kindest

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/moby/term"

	"go.uber.org/zap"

	"github.com/midcontinentcontrols/kindest/pkg/logger"
)

func buildDocker(
	ctx context.Context,
	spec *BuildSpec,
	dest string,
	tag string,
	buildContext []byte,
	relativeDockerfile string,
	options *BuildOptions,
	log logger.Logger,
) error {
	cli, err := client.NewClientWithOpts()
	if err != nil {
		return fmt.Errorf("NewClientWithOpts: %v", err)
	}
	buildArgs := make(map[string]*string)
	for _, arg := range spec.BuildArgs {
		buildArgs[arg.Name] = &arg.Value
	}
	repo := options.Repository
	if repo != "" && !strings.HasSuffix(repo, "/") {
		repo += "/"
	}
	buildArgs["KINDEST_REPOSITORY"] = &repo
	buildArgs["KINDEST_TAG"] = &tag
	if len(options.BuildArgs) > 0 {
		for _, arg := range options.BuildArgs {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid build arg: %s", arg)
			}
			buildArgs[parts[0]] = &parts[1]
			log.Debug("build arg",
				zap.String("name", parts[0]),
				zap.String("value", parts[1]))
		}
	}
	resp, err := cli.ImageBuild(
		ctx,
		bytes.NewReader(buildContext),
		dockertypes.ImageBuildOptions{
			NoCache:    options.NoCache,
			Dockerfile: relativeDockerfile,
			BuildArgs:  buildArgs,
			Squash:     options.Squash,
			Tags:       []string{dest},
			Target:     spec.Target,
		},
	)
	if err != nil {
		return fmt.Errorf("ImageBuild: %v", err)
	}
	var termFd uintptr
	var isTerm bool
	var output io.Writer
	if options.Verbose {
		termFd, isTerm = term.GetFdInfo(os.Stderr)
		output = os.Stderr
	} else {
		output = bytes.NewBuffer(nil)
	}
	if err := jsonmessage.DisplayJSONMessagesStream(
		resp.Body,
		output,
		termFd,
		isTerm,
		nil,
	); err != nil {
		return err
	}
	if !options.NoPush && !spec.SkipPush {
		authConfig, err := RegistryAuthFromEnv(dest)
		if err != nil {
			return fmt.Errorf("RegistryAuthFromEnv: %v", err)
		}
		log.Info("Pushing image", zap.String("username", authConfig.Username))
		authBytes, err := json.Marshal(authConfig)
		if err != nil {
			return err
		}
		registryAuth := base64.URLEncoding.EncodeToString(authBytes)
		resp, err := cli.ImagePush(
			context.TODO(),
			dest,
			dockertypes.ImagePushOptions{
				RegistryAuth: registryAuth,
			},
		)
		if err != nil {
			return fmt.Errorf("ImagePush: %v", err)
		}
		if err := jsonmessage.DisplayJSONMessagesStream(
			resp,
			output,
			termFd,
			isTerm,
			nil,
		); err != nil {
			return err
		}
	} else {
		log.Debug("Skipping push",
			zap.Bool("options.noPush", options.NoPush),
			zap.Bool("spec.SkipPush", spec.SkipPush))
	}
	return nil
}
