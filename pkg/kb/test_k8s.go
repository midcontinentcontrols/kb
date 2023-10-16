package kb

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/google/uuid"
	"github.com/midcontinentcontrols/kb/pkg/logger"
	"github.com/midcontinentcontrols/kb/pkg/util"
	"go.uber.org/zap"
)

func (t *TestSpec) runKubernetes(
	kubeContext string,
	repository string,
	namespace string,
	args []string,
	tag string,
	imagePullPolicy string,
	timeout time.Duration,
	verbose bool,
	log logger.Logger,
) error {
	fmt.Printf("%#v\n", t.Env.Kubernetes)
	client, _, err := util.ClientsetForContext(kubeContext)
	if err != nil {
		return err
	}
	log.Debug("Checking RBAC...")
	if err := createTestRBAC(client, log); err != nil {
		return err
	}
	log.Debug("RBAC resources configured")
	image := util.SanitizeImageName(repository, t.Build.Name, tag)
	if namespace == "" {
		namespace = "default"
	}
	pods := client.CoreV1().Pods(namespace)
	log = log.With(
		zap.String("t.Name", t.Name),
		zap.String("namespace", namespace),
		zap.String("image", image),
		zap.String("imagePullSecret", t.Env.Kubernetes.ImagePullSecret))
	if err := deleteOldPods(pods, t.Name, log); err != nil {
		return err
	}
	log.Debug("Creating test pod",
		zap.String("imagePullPolicy", imagePullPolicy))
	podName := t.Name + "-" + uuid.New().String()[:8]
	var env []corev1.EnvVar
	for _, v := range t.Variables {
		log.Debug("Adding variable to pod",
			zap.String("name", v.Name),
			zap.String("value", v.Value))
		env = append(env, corev1.EnvVar{
			Name:  v.Name,
			Value: v.Value,
		})
	}
	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		name := parts[0]
		var value string
		if len(parts) == 2 {
			value = parts[1]
		}
		found := false
		for i, e := range env {
			if e.Name == name {
				log.Debug("Overriding pod variable",
					zap.String("name", name),
					zap.String("value", value))
				env[i].Value = value
				found = true
				break
			}
		}
		if !found {
			log.Debug("Adding variable to pod",
				zap.String("name", name),
				zap.String("value", value))
			env = append(env, corev1.EnvVar{
				Name:  name,
				Value: value,
			})
		}
	}
	var imagePullSecrets []corev1.LocalObjectReference
	if t.Env.Kubernetes.ImagePullSecret != "" {
		log.Debug("Using imagePullSecret",
			zap.String("secretName", "regcred"))
		imagePullSecrets = []corev1.LocalObjectReference{{
			Name: t.Env.Kubernetes.ImagePullSecret,
		}}
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels: map[string]string{
				"kb": t.Name,
			},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: "kb-test",
			RestartPolicy:      corev1.RestartPolicyNever,
			ImagePullSecrets:   imagePullSecrets,
			Containers: []corev1.Container{{
				Name:            t.Name,
				Image:           image,
				ImagePullPolicy: corev1.PullPolicy(imagePullPolicy),
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
	start := time.Now()
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
				if err := tailLogsKubectl(pod.Name, pod.Namespace); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unexpected phase '%s'", pod.Status.Phase)
		}
		time.Sleep(delay)
		continue
	}
}

func tailLogsClienset(
	pods v1.PodInterface,
	pod *corev1.Pod,
) error {
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
	return nil
}

func tailLogsKubectl(
	name string,
	namespace string,
) error {
	cmd := exec.Command(
		getKubectlCommand(),
		"logs",
		"-f",
		"-n", namespace,
		name,
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func getKubectlCommand() string {
	if v, ok := os.LookupEnv("KUBECTL"); ok {
		return v
	}
	return "kubectl"
}
