package test

import (
	"os"
	"testing"

	"github.com/docker/docker/client"
	"github.com/midcontinentcontrols/kindest/pkg/logger"

	"github.com/stretchr/testify/require"
)

func NewDockerClient(t *testing.T) client.APIClient {
	cli, err := client.NewEnvClient()
	require.NoError(t, err)
	return cli
}

func NewTestLogger() logger.Logger {
	if os.Getenv("DEBUG") == "1" {
		return logger.NewZapLoggerFromEnv()
	}
	return logger.NewFakeLogger()
}
