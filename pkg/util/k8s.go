package util

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	restclient "k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var DefaultWaitTimeout = 60 * time.Second

func HomeDir() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE") // windows
}

func buildConfigFromFlags(context, kubeconfigPath string) (*rest.Config, error) {
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{
			CurrentContext: context,
		}).ClientConfig()
}

// ClientsetForContext creates an official kubernetes client
func ClientsetForContext(kubeContext string) (*kubernetes.Clientset, *restclient.Config, error) {
	// TODO: in-cluster config
	kubeConfigPath := filepath.Join(HomeDir(), ".kube", "config")
	config, err := buildConfigFromFlags(kubeContext, kubeConfigPath)
	if err != nil {
		return nil, nil, err
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, err
	}
	return client, config, nil
}

// CreateKubeClient creates a controller-runtime kubernetes client.
// This client is usually superior to the other, and is often preferred.
func CreateKubeClient(kubeContext string) (client.Client, error) {
	var cfg *rest.Config
	var err error
	if kubeContext != "" {
		cfg, err = config.GetConfigWithContext(kubeContext)
	} else {
		cfg, err = config.GetConfig()
	}
	if err != nil {
		return nil, err
	}
	return client.New(cfg, client.Options{})
}

func WaitForDaemonSet(
	cl client.Client,
	name string,
	namespace string,
) error {
	return WaitForDaemonSet2(
		cl,
		name,
		namespace,
		DefaultWaitTimeout,
	)
}

func WaitForDaemonSet2(
	cl client.Client,
	name string,
	namespace string,
	timeout time.Duration,
) error {
	delay := 3 * time.Second
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		daemonSet := &appsv1.DaemonSet{}
		if err := cl.Get(
			context.TODO(),
			types.NamespacedName{Name: name, Namespace: namespace},
			daemonSet,
		); err == nil {
			if daemonSet.Status.CurrentNumberScheduled == daemonSet.Status.DesiredNumberScheduled {
				// All nodes are ready!
				return nil
			}
		} else if !errors.IsNotFound(err) {
			return err
		}
		time.Sleep(delay)
	}
	return nil
}

func WaitForStatefulSet(
	cl client.Client,
	name string,
	namespace string,
) error {
	return WaitForStatefulSet2(
		cl,
		name,
		namespace,
		DefaultWaitTimeout,
	)
}

func WaitForStatefulSet2(
	cl client.Client,
	name string,
	namespace string,
	timeout time.Duration,
) error {
	delay := 3 * time.Second
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		statefulSet := &appsv1.StatefulSet{}
		if err := cl.Get(
			context.TODO(),
			types.NamespacedName{Name: name, Namespace: namespace},
			statefulSet,
		); err == nil {
			if statefulSet.Status.ReadyReplicas == statefulSet.Status.Replicas {
				return nil
			}
		} else if !errors.IsNotFound(err) {
			return err
		}
		time.Sleep(delay)
	}
	return nil
}

func WaitForDeployment(
	cl client.Client,
	name string,
	namespace string,
) error {
	return WaitForDeployment2(
		cl,
		name,
		namespace,
		DefaultWaitTimeout,
	)
}

func WaitForDeployment2(
	cl client.Client,
	name string,
	namespace string,
	timeout time.Duration,
) error {
	// TODO: return error for status CrashLoopBackOff
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

func WaitForPod(name, namespace string, client *kubernetes.Clientset, log logger.Logger) error {
	timeout := time.Second * 120
	delay := time.Second
	start := time.Now()
	pods := client.CoreV1().Pods(namespace)
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); {
		pod, err := pods.Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		switch pod.Status.Phase {
		case corev1.PodPending:
			log.Info("Waiting on pod",
				zap.String("elapsed", time.Since(start).String()),
				zap.String("timeout", timeout.String()))
			time.Sleep(delay)
			continue
		case corev1.PodRunning:
			return nil
		default:
			return fmt.Errorf("unexpected pod phase '%s' for %s.%s", pod.Status.Phase, pod.Name, pod.Namespace)
		}
	}
	return fmt.Errorf("pod failed to be Ready within %s", timeout.String())
}
