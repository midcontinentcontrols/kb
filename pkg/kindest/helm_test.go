package kindest

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestHelmErrMissingChartYaml(t *testing.T) {
	transient := os.Getenv("KINDEST_PERSISTENT") != "1"
	var kind string
	if !transient {
		kind = "kindest-helm"
	}
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	chartPath := filepath.Join(rootPath, "chart")
	require.NoError(t, os.MkdirAll(chartPath, 0766))
	valuesYaml := `foo: bar
baz: bal`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(chartPath, "values.yaml"),
		[]byte(valuesYaml),
		0644,
	))
	script := `#!/bin/bash
set -euo pipefail
kubectl get deployment -n bar
if [ -z "$(kubectl get deployment -n bar | grep foo-deployment)" ]; then
	echo "foo-deployment not found"
	exit 2
fi
echo "Chart was installed correctly!"`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "script"),
		[]byte(script),
		0644,
	))
	dockerfile := fmt.Sprintf(`FROM alpine:3.11.6
RUN apk add --no-cache wget bash
ENV KUBECTL=v1.17.0
RUN wget -O /usr/local/bin/kubectl https://storage.googleapis.com/kubernetes-release/release/${KUBECTL}/bin/linux/amd64/kubectl \
    && chmod +x /usr/local/bin/kubectl \
	&& mkdir /root/.kube
COPY script /script
RUN chmod +x /script
CMD ["tail", "-f", "/dev/null"]`)
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	manifest := `apiVersion: v1
kind: Namespace
metadata:
  name: test`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "test.yaml"),
		[]byte(manifest),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`build:
  name: test/%s
test:
  - name: basic
    env:
      kubernetes:
        charts:
          - releaseName: foo
            namespace: bar
            name: chart/
            values:
              foo: bologna
    build:
      name: test/%s-test
      dockerfile: Dockerfile
      command: ["/script"]
`, name, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	err := Test(
		&TestOptions{
			File:       specPath,
			NoRegistry: true,
			Transient:  transient,
			Kind:       kind,
		},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing Chart.yaml at ")
}

func TestHelmErrMissingValuesYaml(t *testing.T) {
	transient := os.Getenv("KINDEST_PERSISTENT") != "1"
	var kind string
	if !transient {
		kind = "kindest-helm"
	}
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	chartPath := filepath.Join(rootPath, "chart")
	require.NoError(t, os.MkdirAll(chartPath, 0766))
	chartYaml := `apiVersion: v2
description: helm test for kindest
name: kindest-helm-test
version: 0.0.1
maintainers:
- name: Tom Havlik
  email: thavlik@midcontinentcontrols.com
  url: https://midcontinentcontrols.com`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(chartPath, "Chart.yaml"),
		[]byte(chartYaml),
		0644,
	))
	script := `#!/bin/bash
set -euo pipefail
kubectl get deployment -n bar
if [ -z "$(kubectl get deployment -n bar | grep foo-deployment)" ]; then
	echo "foo-deployment not found"
	exit 2
fi
echo "Chart was installed correctly!"`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "script"),
		[]byte(script),
		0644,
	))
	dockerfile := fmt.Sprintf(`FROM alpine:3.11.6
RUN apk add --no-cache wget bash
ENV KUBECTL=v1.17.0
RUN wget -O /usr/local/bin/kubectl https://storage.googleapis.com/kubernetes-release/release/${KUBECTL}/bin/linux/amd64/kubectl \
    && chmod +x /usr/local/bin/kubectl \
	&& mkdir /root/.kube
COPY script /script
RUN chmod +x /script
CMD ["tail", "-f", "/dev/null"]`)
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	manifest := `apiVersion: v1
kind: Namespace
metadata:
  name: test`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "test.yaml"),
		[]byte(manifest),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`build:
  name: test/%s
test:
  - name: basic
    env:
      kubernetes:
        charts:
          - releaseName: foo
            namespace: bar
            name: chart/
            values:
              foo: bologna
    build:
      name: test/%s-test
      dockerfile: Dockerfile
      command: ["/script"]
`, name, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	err := Test(
		&TestOptions{
			File:       specPath,
			NoRegistry: true,
			Transient:  transient,
			Kind:       kind,
		},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing values.yaml at ")
}

func TestHelmLocalChartInstall(t *testing.T) {
	transient := os.Getenv("KINDEST_PERSISTENT") != "1"
	var kind string
	if !transient {
		kind = "kindest-helm"
	}
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	chartPath := filepath.Join(rootPath, "chart")
	require.NoError(t, os.MkdirAll(chartPath, 0766))
	valuesYaml := `foo: bar
baz: bal`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(chartPath, "values.yaml"),
		[]byte(valuesYaml),
		0644,
	))
	chartYaml := `apiVersion: v2
description: "helm test for kindest"
name: kindest-helm-test
version: 0.0.1
maintainers:
  - name: Tom Havlik
    email: thavlik@midcontinentcontrols.com
    url: https://midcontinentcontrols.com`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(chartPath, "Chart.yaml"),
		[]byte(chartYaml),
		0644,
	))
	templatesPath := filepath.Join(chartPath, "templates")
	require.NoError(t, os.MkdirAll(templatesPath, 0766))
	deploymentYaml := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}-deployment
  labels:
    chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
spec:
  selector:
    matchLabels:
      app: {{ .Release.Name }}-deployment
  template:
    metadata:
      labels:
        app: {{ .Release.Name }}-deployment
    spec:
      containers:
        - name: foo
          imagePullPolicy: Never
          image: test/%s:latest`, name)
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(templatesPath, "deployment.yaml"),
		[]byte(deploymentYaml),
		0644,
	))
	script := `#!/bin/bash
set -euo pipefail
kubectl get deployment -n bar
if [ -z "$(kubectl get deployment -n bar | grep foo-deployment)" ]; then
	echo "foo-deployment not found"
	exit 2
fi
echo "Chart was installed correctly!"`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "script"),
		[]byte(script),
		0644,
	))
	dockerfile := fmt.Sprintf(`FROM alpine:3.11.6
RUN apk add --no-cache wget bash
ENV KUBECTL=v1.17.0
RUN wget -O /usr/local/bin/kubectl https://storage.googleapis.com/kubernetes-release/release/${KUBECTL}/bin/linux/amd64/kubectl \
    && chmod +x /usr/local/bin/kubectl \
	&& mkdir /root/.kube
COPY script /script
RUN chmod +x /script
CMD ["tail", "-f", "/dev/null"]`)
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	manifest := `apiVersion: v1
kind: Namespace
metadata:
  name: test`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "test.yaml"),
		[]byte(manifest),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`build:
  name: test/%s
test:
  - name: basic
    env:
      kubernetes:
        charts:
          - namespace: bar
            releaseName: foo
            name: chart/
            values:
              foo: bal
    build:
      name: test/%s-test
      dockerfile: Dockerfile
      command: ["/script"]
`, name, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	require.NoError(t, Test(
		&TestOptions{
			File:       specPath,
			NoRegistry: true,
			Transient:  transient,
			Kind:       kind,
		},
	))
}

func TestHelmLocalChartUpgrade(t *testing.T) {
	transient := os.Getenv("KINDEST_PERSISTENT") != "1"
	var kind string
	if !transient {
		kind = "kindest-helm"
	}
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	chartPath := filepath.Join(rootPath, "chart")
	require.NoError(t, os.MkdirAll(chartPath, 0766))
	valuesYaml := `foo: bar
baz: bal`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(chartPath, "values.yaml"),
		[]byte(valuesYaml),
		0644,
	))
	chartYaml := `apiVersion: v2
description: "helm test for kindest"
name: kindest-helm-test
version: 0.0.1
maintainers:
  - name: Tom Havlik
    email: thavlik@midcontinentcontrols.com
    url: https://midcontinentcontrols.com`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(chartPath, "Chart.yaml"),
		[]byte(chartYaml),
		0644,
	))
	templatesPath := filepath.Join(chartPath, "templates")
	require.NoError(t, os.MkdirAll(templatesPath, 0766))
	deploymentYaml := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}-deployment
  labels:
    chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
spec:
  selector:
    matchLabels:
      app: {{ .Release.Name }}-deployment
  template:
    metadata:
      labels:
        app: {{ .Release.Name }}-deployment
    spec:
      containers:
        - name: foo
          imagePullPolicy: Never
          image: test/%s:latest
          env:
            - name: FOO
              value: BAR`, name)
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(templatesPath, "deployment.yaml"),
		[]byte(deploymentYaml),
		0644,
	))
	script := `#!/bin/bash
set -euo pipefail
kubectl get deployment -n bar
if [ -z "$(kubectl get deployment -n bar | grep foo-deployment)" ]; then
	echo "foo-deployment not found"
	exit 2
fi
echo "Chart was installed correctly!"`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "script"),
		[]byte(script),
		0644,
	))
	dockerfile := fmt.Sprintf(`FROM alpine:3.11.6
RUN apk add --no-cache wget bash
ENV KUBECTL=v1.17.0
RUN wget -O /usr/local/bin/kubectl https://storage.googleapis.com/kubernetes-release/release/${KUBECTL}/bin/linux/amd64/kubectl \
    && chmod +x /usr/local/bin/kubectl \
	&& mkdir /root/.kube
COPY script /script
RUN chmod +x /script
CMD ["tail", "-f", "/dev/null"]`)
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	manifest := `apiVersion: v1
kind: Namespace
metadata:
  name: test`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "test.yaml"),
		[]byte(manifest),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`build:
  name: test/%s
test:
  - name: basic
    env:
      kubernetes:
        charts:
          - namespace: bar
            releaseName: foo
            name: chart/
            values:
              foo: bal
    build:
      name: test/%s-test
      dockerfile: Dockerfile
      command: ["/script"]
`, name, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	require.NoError(t, Test(
		&TestOptions{
			File:       specPath,
			NoRegistry: true,
			Transient:  false,
			Kind:       kind,
		},
	))
	deploymentYaml = fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}-deployment
  labels:
    chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
spec:
  selector:
    matchLabels:
      app: {{ .Release.Name }}-deployment
  template:
    metadata:
      labels:
        app: {{ .Release.Name }}-deployment
    spec:
      containers:
        - name: foo
          imagePullPolicy: Never
          image: test/%s:latest
          env:
            - name: FOO
              value: BAZ`, name)
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(templatesPath, "deployment.yaml"),
		[]byte(deploymentYaml),
		0644,
	))
	require.NoError(t, Test(
		&TestOptions{
			File:       specPath,
			NoRegistry: true,
			Transient:  false,
			Kind:       kind,
		},
	))
}
