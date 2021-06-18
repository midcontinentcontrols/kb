package kindest

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/midcontinentcontrols/kindest/pkg/test"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
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

func runTest(
	t *testing.T,
	files func(name string) map[string]interface{},
	f func(t *testing.T, rootPath string),
) {
	name := test.RandomTestName()
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, test.CreateFiles(files(name), rootPath))
	f(t, rootPath)
}
