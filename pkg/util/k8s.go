package util

import (
	"context"
	"fmt"
	"time"

	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
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
