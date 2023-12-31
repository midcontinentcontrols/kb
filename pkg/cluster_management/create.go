package cluster_management

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/docker/docker/client"
	"github.com/midcontinentcontrols/kb/pkg/kubeconfig"
	"github.com/midcontinentcontrols/kb/pkg/logger"
	"github.com/midcontinentcontrols/kb/pkg/registry"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/kind/pkg/cluster"
)

func ClientForKindCluster(name string, provider *cluster.Provider) (*kubernetes.Clientset, string, error) {
	if err := kubeconfig.Save(provider, name, "", false); err != nil {
		return nil, "", err
	}
	kubeConfig, err := provider.KubeConfig(name, false)
	if err != nil {
		return nil, "", err
	}
	config, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeConfig))
	if err != nil {
		return nil, "", err
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, "", err
	}
	return client, "kind-" + name, nil
}

func GenerateKindConfig(regName string, regPort int) string {
	return fmt.Sprintf(`kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
    [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:%d"]
    endpoint = ["http://%s:%d"]`, regPort, regName, regPort)
}

func WaitForCluster(client *kubernetes.Clientset, log logger.Logger) error {
	timeout := time.Second * 120
	delay := time.Second
	start := time.Now()
	good := false
	deadline := time.Now().Add(timeout)
	p := client.CoreV1().Pods("kube-system")
	for time.Now().Before(deadline) {
		pods, err := p.List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return err
		}
		count := 0
		for _, pod := range pods.Items {
			switch pod.Status.Phase {
			case corev1.PodPending:
				count++
			case corev1.PodRunning:
				continue
			default:
				return fmt.Errorf("unexpected pod phase '%s' for %s.%s", pod.Status.Phase, pod.Name, pod.Namespace)
			}
		}
		if count == 0 {
			good = true
			break
		}
		total := len(pods.Items)
		log.Info("Waiting on pods in kube-system",
			zap.Int("numReady", total-count),
			zap.Int("numPods", total),
			zap.String("elapsed", time.Since(start).String()),
			zap.String("timeout", timeout.String()))
		time.Sleep(delay)
	}
	if !good {
		return fmt.Errorf("pods in kube-system failed to be Ready within %s", timeout.String())
	}
	good = false
	sa := client.CoreV1().ServiceAccounts("default")
	for time.Now().Before(deadline) {
		if _, err := sa.Get(
			context.TODO(),
			"default",
			metav1.GetOptions{},
		); err != nil {
			if errors.IsNotFound(err) {
				log.Info("Waiting on default serviceaccount",
					zap.String("elapsed", time.Since(start).String()),
					zap.String("timeout", timeout.String()))
				time.Sleep(delay)
				continue
			}
			return err
		}
		good = true
		break
	}
	if !good {
		return fmt.Errorf("default serviceaccount failed to appear within %v", timeout.String())
	}
	return nil
}

func ClusterExists(name string) (bool, error) {
	p := cluster.NewProvider()
	existing, err := p.List()
	if err != nil {
		return false, fmt.Errorf("kind: %v", err)
	}
	for _, n := range existing {
		if n == name {
			return true, nil
		}
	}
	return false, nil
}

func CreateCluster(name string, log logger.Logger) (string, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return "", fmt.Errorf("docker: %v", err)
	}
	exists, err := ClusterExists(name)
	if err != nil {
		return "", err
	}
	p := cluster.NewProvider()
	if !exists {
		kindConfig := GenerateKindConfig("kind-registry", 5000)
		config := cluster.CreateWithRawConfig([]byte(kindConfig))
		if err := p.Create(name, config); err != nil {
			//  && !strings.Contains(err.Error(), fmt.Sprintf(`node(s) already exist for a cluster with the name "%s"`, name))
			return "", fmt.Errorf("kind: %v", err)
		}
	}
	client, kubeContext, err := ClientForKindCluster(name, p)
	if err != nil {
		return "", err
	}
	if err := WaitForCluster(client, log); err != nil {
		return "", err
	}
	if err := registry.EnsureLocalRegistryRunning(cli, true, log); err != nil {
		return "", fmt.Errorf("registry: %v", err)
	}
	return kubeContext, nil
}

func DeleteCluster(name string) error {
	p := cluster.NewProvider()
	if err := p.Delete(name, ""); err != nil {
		return fmt.Errorf("kind: %v", err)
	}
	return nil
}
