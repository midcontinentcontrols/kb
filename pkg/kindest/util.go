package kindest

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func WaitForDeployment(
	cl client.Client,
	name string,
	namespace string,
) error {
	return WaitForDeployment2(
		cl,
		name,
		namespace,
		30*time.Second,
	)
}

func WaitForDeployment2(
	cl client.Client,
	name string,
	namespace string,
	timeout time.Duration,
) error {
	delay := 3 * time.Second
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		deployment := &appsv1.Deployment{}
		if err := cl.Get(
			context.TODO(),
			types.NamespacedName{Name: name, Namespace: namespace},
			deployment,
		); err == nil {
			var replicas int32 = 1
			if deployment.Spec.Replicas != nil {
				replicas = *deployment.Spec.Replicas
			}
			if deployment.Status.AvailableReplicas >= replicas {
				// All replicas are available
				return nil
			}
		} else if err != nil && !errors.IsNotFound(err) {
			return err
		}
		time.Sleep(delay)
	}
	return nil
}

func sanitizeImageName(host, image, tag string) string {
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

type testEnv struct {
	files map[string]interface{}
}

func createFiles(files map[string]interface{}, dir string) error {
	if err := os.MkdirAll(dir, 0766); err != nil {
		return err
	}
	for k, v := range files {
		path := filepath.Join(dir, k)
		if m, ok := v.(map[string]interface{}); ok {
			if err := createFiles(m, path); err != nil {
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

func runTest(
	t *testing.T,
	files func(name string) map[string]interface{},
	f func(t *testing.T, rootPath string),
) {
	name := RandomTestName()
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, createFiles(files(name), rootPath))
	f(t, rootPath)
}

func CreateKubeClient(t *testing.T) client.Client {
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err)
	cl, err := client.New(config, client.Options{})
	require.NoError(t, err)
	return cl
}
