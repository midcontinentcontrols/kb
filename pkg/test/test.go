package test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/docker/client"
	"github.com/google/uuid"
	"github.com/midcontinentcontrols/kindest/pkg/cluster_management"
	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func RandomTestName() string {
	return "test-" + uuid.New().String()[:8]
}

func CreateFiles(dir string, files map[string]interface{}) error {
	if err := os.MkdirAll(dir, 0766); err != nil {
		return err
	}
	for k, v := range files {
		path := filepath.Join(dir, k)
		if m, ok := v.(map[string]interface{}); ok {
			if err := CreateFiles(path, m); err != nil {
				return fmt.Errorf("%s: %v", path, err)
			}
		} else if s, ok := v.(string); ok {
			if err := ioutil.WriteFile(
				path,
				[]byte(s),
				0644,
			); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("unknown type for %s", path)
		}
	}
	return nil
}

func CreateKubeClient(t *testing.T) k8sclient.Client {
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err)
	cl, err := k8sclient.New(config, k8sclient.Options{})
	require.NoError(t, err)
	return cl
}

func WithTemporaryModule(t *testing.T, f func(name string, rootPath string)) {
	name := RandomTestName()
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0644))
	defer func() {
		require.NoError(t, os.RemoveAll(rootPath))
	}()
	f(name, rootPath)
}

func WithTemporaryCluster(name string, t *testing.T, log logger.Logger, f func(cl k8sclient.Client)) {
	_, err := cluster_management.CreateCluster(name, log)
	require.NoError(t, err)
	defer cluster_management.DeleteCluster(name)
	cl := CreateKubeClient(t)
	f(cl)
}

func NewDockerClient(t *testing.T) client.APIClient {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	require.NoError(t, err)
	return cli
}

func NewTestLogger() logger.Logger {
	if os.Getenv("DEBUG") == "1" {
		return logger.NewZapLoggerFromEnv()
	}
	return logger.NewFakeLogger()
}
