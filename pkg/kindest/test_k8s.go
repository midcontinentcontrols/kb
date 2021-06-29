package kindest

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/google/uuid"
	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"github.com/midcontinentcontrols/kindest/pkg/util"
	"go.uber.org/zap"
)

func (t *TestSpec) runKubernetes(
	kubeContext string,
	repository string,
	namespace string,
	timeout time.Duration,
	verbose bool,
	log logger.Logger,
) error {
	client, _, err := util.ClientsetForContext(kubeContext)
	if err != nil {
		return err
	}
	start := time.Now()
	log.Debug("Checking RBAC...")
	if err := createTestRBAC(client, log); err != nil {
		return err
	}
	image := util.SanitizeImageName(repository, t.Build.Name, "latest")
	imagePullPolicy := corev1.PullAlways
	if namespace == "" {
		namespace = "default"
	}
	pods := client.CoreV1().Pods(namespace)
	log = log.With(
		zap.String("t.Name", t.Name),
		zap.String("namespace", namespace),
		zap.String("image", image),
	)
	if err := deleteOldPods(pods, t.Name, log); err != nil {
		return err
	}
	log.Debug("Creating test pod")
	podName := t.Name + "-" + uuid.New().String()[:8]
	var env []corev1.EnvVar
	for _, v := range t.Variables {
		env = append(env, corev1.EnvVar{
			Name:  v.Name,
			Value: v.Value,
		})
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels: map[string]string{
				"kindest": t.Name,
			},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: "kindest-test",
			RestartPolicy:      corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:            t.Name,
				Image:           image,
				ImagePullPolicy: imagePullPolicy,
				Command:         t.Build.Command,
				Env:             env,
			}},
		},
	}
	if pod, err = pods.Create(
		context.TODO(),
		pod,
		metav1.CreateOptions{},
	); err != nil {
		return err
	}
	log.Debug("Created test pod", zap.String("name", pod.Name))
	delay := time.Second
	start = time.Now()
	scheduled := false
	for {
		pod, err = pods.Get(
			context.TODO(),
			podName,
			metav1.GetOptions{},
		)
		if err != nil {
			return err
		}
		switch pod.Status.Phase {
		case corev1.PodPending:
			if !scheduled {
				for _, condition := range pod.Status.Conditions {
					if condition.Status == "PodScheduled" {
						scheduled = true
					}
				}
			}
			log.Info("Still waiting on pod",
				zap.String("phase", string(pod.Status.Phase)),
				zap.Bool("scheduled", scheduled),
				zap.String("elapsed", time.Since(start).String()))
			time.Sleep(delay)
			continue
		case corev1.PodRunning:
			fallthrough
		case corev1.PodSucceeded:
			fallthrough
		case corev1.PodFailed:
			for _, status := range pod.Status.ContainerStatuses {
				if status.State.Terminated != nil {
					if status.State.Terminated.ExitCode == 0 { // && status.State.Terminated.Reason == "Completed" {
						// Success condition
						return nil
					}
					return fmt.Errorf("exit code %d", status.State.Terminated.ExitCode)
				}
			}
			if verbose {
				req := pods.GetLogs(pod.Name, &corev1.PodLogOptions{
					Follow: true,
				})
				r, err := req.Stream(context.TODO())
				if err != nil {
					return err
				}
				if _, err := io.Copy(os.Stdout, r); err != nil {
					return fmt.Errorf("copy: %v", err)
				}
			}
		default:
			return fmt.Errorf("unexpected phase '%s'", pod.Status.Phase)
		}
		time.Sleep(delay)
		continue
	}
}
