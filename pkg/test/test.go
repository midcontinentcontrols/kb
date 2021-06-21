package test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/docker/docker/client"
	"github.com/google/uuid"
	"github.com/midcontinentcontrols/kindest/pkg/cluster_management"
	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"github.com/midcontinentcontrols/kindest/pkg/util"
	"github.com/stretchr/testify/require"
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

func CreateKubeClient(t *testing.T, kubeContext string) k8sclient.Client {
	require.NotEmpty(t, kubeContext)
	cl, err := util.CreateKubeClient(kubeContext)
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

var numClustersCreated int32

func WithTemporaryCluster(t *testing.T, name string, log logger.Logger, f func(kubeContext string, cl k8sclient.Client)) {
	var kubeContext string
	var ok bool
	if kubeContext, ok = os.LookupEnv("KUBECONTEXT"); !ok {
		var err error
		kubeContext, err = cluster_management.CreateCluster(name, log)
		require.NoError(t, err)
		defer cluster_management.DeleteCluster(name)
		if atomic.AddInt32(&numClustersCreated, 1) == 2 {
			fmt.Println("Warning: a transient cluster is being created for each test. Set the KUBECONTEXT environment variable to significantly reduce test execution time.")
		}
	} else if atomic.AddInt32(&numClustersCreated, -1) == -1 {
		fmt.Printf("Using persistent kube context %s\n", kubeContext)
	}
	f(kubeContext, CreateKubeClient(t, kubeContext))
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
