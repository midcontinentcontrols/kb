package kindest

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"github.com/midcontinentcontrols/kindest/pkg/util"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func buildKaniko(
	spec *BuildSpec,
	dest string,
	buildContext []byte,
	relativeDockerfile string,
	options *BuildOptions,
	log logger.Logger,
) error {
	cl, config, err := util.ClientsetForContext(options.Context)
	if err != nil {
		return err
	}
	namespace := options.Namespace
	if namespace == "" {
		namespace = "default"
	}
	pod := kanikoPod(namespace)
	if _, err := cl.CoreV1().Pods(namespace).Create(
		context.TODO(),
		pod,
		metav1.CreateOptions{},
	); err != nil {
		return err
	}
	defer func() {
		if err := cl.CoreV1().Pods(namespace).Delete(
			context.TODO(),
			pod.Name,
			metav1.DeleteOptions{},
		); err != nil {
			log.Error(
				"failed to delete kaniko pod",
				zap.String("name", pod.Name),
				zap.String("namespace", namespace),
				zap.Error(err))
		}
	}()
	buildContext, err = util.GZipCompress(buildContext)
	if err != nil {
		return err
	}
	if err := util.WaitForPod(pod.Name, pod.Namespace, cl, log); err != nil {
		return err
	}
	if !options.NoPush {
		// The pod needs push credentials
		fmt.Println("Copying docker config to kaniko pod...")
		if err := util.CopyDockerConfigToPod(
			pod,
			cl,
			config,
		); err != nil {
			return fmt.Errorf("CopyDockerConfigToPod: %v", err)
		}
		fmt.Println("Successfully copied it over")
		log.Debug("Copied docker config to kaniko pod")
	}
	time.Sleep(time.Minute)
	command := []string{
		"/kaniko/executor",
		"--dockerfile=" + relativeDockerfile,
		"--context=tar://stdin",
	}
	if options.NoPush {
		command = append(command, "--no-push")
	} else {
		dest := util.SanitizeImageName(
			options.Repository,
			spec.Name,
			options.Tag,
		)
		command = append(command, "--destination="+dest)
	}
	for _, buildArg := range spec.BuildArgs {
		command = append(command, fmt.Sprintf("--build-arg=%s=%s", buildArg.Name, buildArg.Value))
	}
	var stdout, stderr io.Writer
	if options.Verbose {
		stdout = os.Stdout
		stderr = os.Stderr
	} else {
		stdout = bytes.NewBuffer(nil)
		stderr = bytes.NewBuffer(nil)
	}
	if err := util.ExecInPod(
		cl,
		config,
		pod,
		&corev1.PodExecOptions{
			Command: command,
			Stdin:   true,
			Stdout:  true,
			Stderr:  true,
			TTY:     false,
		},
		bytes.NewReader(buildContext),
		stdout,
		stderr,
	); err != nil {
		return fmt.Errorf("kaniko: %v", err)
	}
	return nil
}

func kanikoPod(namespace string) *corev1.Pod {
	command := "set -e; "
	command += ` echo "Tailing null..."; tail -f /dev/null`
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kaniko-" + uuid.New().String()[:8],
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:            "kaniko",
				Image:           "gcr.io/kaniko-project/executor:debug",
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command: []string{
					"sh",
					"-c",
					command,
				},
			}},
		},
	}
}
