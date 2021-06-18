package kindest

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/midcontinentcontrols/kindest/pkg/test"

	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var chartYaml = `apiVersion: v2
description: "Kindest DevOps for Monorepos (test chart)"
name: kindest
version: 0.0.1
maintainers:
  - name: Tom Havlik
    email: thavlik@midcontinentcontrols.com
    url: https://midcontinentcontrols.com`

// Make sure we encounter an error when Chart.yaml is missing
func TestDeployErrMissingChartYaml(t *testing.T) {
	test.WithTemporaryModule(t, func(name string, rootPath string) {
		log := logger.NewMockLogger(logger.NewFakeLogger())
		pushRepo := getPushRepository()
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
		require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
			"kindest.yaml": specYaml,
			"Dockerfile":   dockerfile,
			"test.yaml":    manifest,
			"script":       script,
			"chart": map[string]interface{}{
				"values.yaml": valuesYaml,
			},
		}))
		module, err := NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
		require.NoError(t, err)
		require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
		test.WithTemporaryCluster(name, t, log, func(cl client.Client) {
			_, err = module.Deploy(&DeployOptions{})
			require.Error(t, err)
			require.Contains(t, err.Error(), "Chart.yaml file is missing")
		})
	})
}

// Try and deploy a basic chart, then ensure the chart resources
// are created appropriately.
func TestDeployChart(t *testing.T) {
	test.WithTemporaryModule(t, func(name string, rootPath string) {
		namespace := name
		pushRepo := getPushRepository()
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
		require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
			"kindest.yaml": specYaml,
			"Dockerfile":   dockerfile,
			"chart": map[string]interface{}{
				"Chart.yaml":  chartYaml,
				"values.yaml": valuesYaml,
				"templates": map[string]interface{}{
					"deployment.yaml": deploymentYaml,
				},
			},
		}))
		log := logger.NewMockLogger(logger.NewFakeLogger())
		module, err := NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
		require.NoError(t, err)
		require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
		test.WithTemporaryCluster(name, t, log, func(cl client.Client) {
			require.NoError(t, cl.Create(context.TODO(), &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}))
			_, err = module.Deploy(&DeployOptions{})
			require.NoError(t, err)
			require.NoError(t, WaitForDeployment(cl, "foo-busybox", namespace))
		})
	})
}

/*
func TestDeployChartRestartImages(t *testing.T) {
	name := test.RandomTestName()
	pushRepo := getPushRepository()
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "set -euo pipefail; echo $MYVARIABLE"]`
	specYaml := fmt.Sprintf(`build:
  name: %s/%s
env:
  kubernetes: {}
test:
  - name: basic
    variables:
      - name: MYVARIABLE
        value: foobarbaz
    env:
      kubernetes: {}
    build:
      name: %s/%s-test
      dockerfile: Dockerfile
`, pushRepo, kindestTestImageName, pushRepo, kindestTestImageName)
	require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
		"kindest.yaml": specYaml,
		"Dockerfile":   dockerfile,
	}))
	log := logger.NewMockLogger(logger.NewFakeLogger())
	p := NewProcess(runtime.NumCPU(), log)
	module, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
	require.NoError(t, err)
	err = module.RunTests(&TestOptions{
		Kind:      name,
		Transient: true,
	}, p, log)
	require.NoError(t, err)
}
*/
