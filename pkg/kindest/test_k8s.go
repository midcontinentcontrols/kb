package kindest

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/google/uuid"
	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"go.uber.org/zap"
)

func (t *TestSpec) runKubernetes(options *TestOptions, log logger.Logger) error {
	client, _, err := clientForContext(options.KubeContext)
	if err != nil {
		return err
	}
	start := time.Now()
	log.Debug("Checking RBAC...")
	if err := createTestRBAC(client, log); err != nil {
		return err
	}
	// TODO: move to deploy phase
	//if err := applyTestManifests(
	//	options.KubeContext,
	//	rootPath,
	//	t.Env.Kubernetes.Resources,
	//); err != nil {
	//	return err
	//}
	//if err := t.installCharts(
	//	rootPath,
	//	options.KubeContext,
	//	options,
	//	log,
	//); err != nil {
	//	return err
	//}
	image := sanitizeImageName(options.Repository, t.Build.Name, "latest")
	imagePullPolicy := corev1.PullAlways
	namespace := options.Namespace
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
	for _, v := range t.Env.Variables {
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
	timeout := 90 * time.Second
	delay := time.Second
	start = time.Now()
	scheduled := false
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline); {
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
						deadline = time.Now().Add(30 * time.Second)
						scheduled = true
					}
				}
			}
			for _, status := range pod.Status.ContainerStatuses {
				if status.State.Terminated != nil {
					if code := status.State.Terminated.ExitCode; code != 0 {
						return fmt.Errorf("pod failed with exit code '%d'", code)
					}
					return nil
				}
				if status.State.Waiting != nil {
					if strings.Contains(status.State.Waiting.Reason, "Err") {
						return fmt.Errorf("pod failed with '%s'", status.State.Waiting.Reason)
					}
				}
			}
			log.Info("Still waiting on pod",
				zap.String("phase", string(pod.Status.Phase)),
				zap.Bool("scheduled", scheduled),
				zap.String("elapsed", time.Since(start).String()),
				zap.String("timeout", timeout.String()))
			time.Sleep(delay)
			continue
		case corev1.PodRunning:
			fallthrough
		case corev1.PodSucceeded:
			fallthrough
		case corev1.PodFailed:
			for _, status := range pod.Status.ContainerStatuses {
				if status.State.Terminated != nil {
					if strings.Contains(status.State.Terminated.Reason, "Err") {
						return ErrTestFailed
					}
				}
			}
			req := pods.GetLogs(pod.Name, &corev1.PodLogOptions{
				Follow: true,
			})
			r, err := req.Stream(context.TODO())
			if err != nil {
				return err
			}
			rd := bufio.NewReader(r)
			for {
				message, err := rd.ReadString('\n')
				if err != nil {
					if err == io.EOF {
						break
					}
					return err
				}
				//log.Info("Test output", zap.String("message", message))
				fmt.Println(message)
			}
		default:
			return fmt.Errorf("unexpected phase '%s'", pod.Status.Phase)
		}
		if pod, err = pods.Get(
			context.TODO(),
			pod.Name,
			metav1.GetOptions{},
		); err != nil {
			return err
		}
		if pod.Status.Phase == corev1.PodRunning {
			time.Sleep(delay)
			log.Warn("Log stream terminated prematurely. Retailing logs...")
			continue
		} else if pod.Status.Phase == corev1.PodSucceeded {
			return nil
		} else if pod.Status.Phase == corev1.PodFailed {
			// This should NOT happen. Container terminated status
			// should exist if the phase is Failed.
			return ErrTestFailed
		} else {
			return fmt.Errorf("unexpected pod phase '%s'", pod.Status.Phase)
		}
	}
	return ErrPodTimeout
	//kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	//config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	//if err != nil {
	//	return fmt.Errorf("create config: %v", err)
	//}
	//_, err = client.New(config, client.Options{})
	//if err != nil {
	//	return fmt.Errorf("create client: %v", err)
	//}
	//panic(cl)
}
