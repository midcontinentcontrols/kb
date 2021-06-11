package kindest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var chartYaml = `apiVersion: v2
description: Kindest DevOps for Monorepos
name: kindest
version: 0.0.1
maintainers:
  - name: Tom Havlik
    email: thavlik@midcontinentcontrols.com
    url: https://midcontinentcontrols.com`

func TestDeploy(t *testing.T) {

}

func TestDeployErrMissingChartYaml(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	pushRepo := getPushRepository()
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	valuesYaml := `foo: bar
baz: bal`
	script := `#!/bin/bash
set -euo pipefail
kubectl get deployment -n bar
if [ -z "$(kubectl get deployment -n bar | grep foo-deployment)" ]; then
	echo "foo-deployment not found"
	exit 2
fi
echo "Chart was installed correctly!"`
	dockerfile := `FROM alpine:3.11.6
RUN apk add --no-cache wget bash
ENV KUBECTL=v1.17.0
RUN wget -O /usr/local/bin/kubectl https://storage.googleapis.com/kubernetes-release/release/${KUBECTL}/bin/linux/amd64/kubectl \
    && chmod +x /usr/local/bin/kubectl \
	&& mkdir /root/.kube
COPY script /script
RUN chmod +x /script
CMD ["tail", "-f", "/dev/null"]`
	manifest := `apiVersion: v1
kind: Namespace
metadata:
  name: test`
	specYaml := fmt.Sprintf(`build:
  name: %s/%s
env:
  kubernetes:
    charts:
      - releaseName: foo
        namespace: bar
        name: chart/
        values:
          foo: bologna
test:
  - name: basic
    env:
      kubernetes: {}
    build:
      name: %s/%s-test
      dockerfile: Dockerfile
      command: ["/script"]
`, pushRepo, kindestTestImageName, pushRepo, kindestTestImageName)
	require.NoError(t, createFiles(map[string]interface{}{
		"kindest.yaml": specYaml,
		"Dockerfile":   dockerfile,
		"test.yaml":    manifest,
		"script":       script,
		"chart": map[string]interface{}{
			"values.yaml": valuesYaml,
		},
	}, rootPath))
	log := logger.NewMockLogger(logger.NewFakeLogger())
	module, err := NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
	require.NoError(t, err)
	require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
	err = module.Deploy(&DeployOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "Chart.yaml file is missing")
}

func TestDeployChart(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	namespace := name
	cl := CreateKubeClient(t)
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	require.NoError(t, cl.Create(context.TODO(), ns))
	defer cl.Delete(context.TODO(), ns)
	pushRepo := getPushRepository()
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := `FROM alpine:3.11.6
CMD ["tail", "-f", "/dev/null"]`
	deploymentYaml := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}-busybox
  labels:
    chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
spec:
  selector:
    matchLabels:
      app: {{ .Release.Name }}-busybox
  template:
    metadata:
      labels:
        app: {{ .Release.Name }}-busybox
    spec:
      containers:
      - name: mycontainer
        imagePullPolicy: Always
        image: {{ .Values.image }}
        command: ["tail", "-f", "/dev/null"]`
	valuesYaml := `image: ""`
	specYaml := fmt.Sprintf(`build:
  name: %s/%s
env:
  kubernetes:
    charts:
      - releaseName: foo
        namespace: %s
        name: chart/
        values:
          image: busybox:latest
`, pushRepo, kindestTestImageName, namespace)
	require.NoError(t, createFiles(map[string]interface{}{
		"kindest.yaml": specYaml,
		"Dockerfile":   dockerfile,
		"chart": map[string]interface{}{
			"Chart.yaml":  chartYaml,
			"values.yaml": valuesYaml,
			"templates": map[string]interface{}{
				"deployment.yaml": deploymentYaml,
			},
		},
	}, rootPath))
	log := logger.NewMockLogger(logger.NewFakeLogger())
	module, err := NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
	require.NoError(t, err)
	require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
	err = module.Deploy(&DeployOptions{})
	require.NoError(t, err)
	require.NoError(t, WaitForDeployment(cl, "foo-busybox", namespace))
}
