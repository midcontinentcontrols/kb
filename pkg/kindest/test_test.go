package kindest

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestNoTests(t *testing.T) {
	specPath := createBasicTestProject(t, "tmp")
	defer os.RemoveAll(filepath.Dir(filepath.Dir(specPath)))
	require.Equal(t, ErrNoTests, Test(
		&TestOptions{
			File: specPath,
		},
	))
}

func TestTestDockerEnv(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := `FROM alpine:latest
CMD ["sh", "-c", "set -eu; echo $MESSAGE"]`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`build:
  name: test/%s
test:
  - name: basic
    env:
      variables:
        - name: MESSAGE
          value: "It works!"
      docker: {}
    build:
      name: midcontinentcontrols/kindest-basic-test
      dockerfile: Dockerfile
`, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	require.NoError(t, Test(
		&TestOptions{
			File: specPath,
		},
	))
}

func TestErrNoTestEnv(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := `FROM alpine:latest
CMD ["sh", "-c", "echo \"Hello, world\""]`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`build:
  name: test/%s
test:
  - name: basic
    build:
      name: midcontinentcontrols/kindest-basic-test
      dockerfile: Dockerfile
`, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	require.Equal(t, ErrNoTestEnv, Test(
		&TestOptions{
			File: specPath,
		},
	))
}

func TestErrMultipleTestEnv(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := `FROM alpine:latest
CMD ["sh", "-c", "echo \"Hello, world\""]`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`build:
  name: test/%s
test:
  - name: basic
    env:
      kind: {}
      docker: {}
    build:
      name: midcontinentcontrols/kindest-basic-test
      dockerfile: Dockerfile
`, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	require.Equal(t, ErrMultipleTestEnv, Test(
		&TestOptions{
			File: specPath,
		},
	))
}

func TestTestError(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := `FROM alpine:latest
CMD ["sh", "-c", "exit 1"]`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`build:
  name: test/%s
test:
  - name: basic
    env:
      docker: {}
    build:
      name: test/%s-test
      dockerfile: Dockerfile
`, name, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	err := Test(
		&TestOptions{
			File: specPath,
		},
	)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "exit code 1"))
}

func TestTestKindEnv(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := `FROM alpine:latest
CMD ["sh", "-c", "echo 'Hello, world!'"]`
	require.NoError(t, ioutil.WriteFile(
		filepath.Join(rootPath, "Dockerfile"),
		[]byte(dockerfile),
		0644,
	))
	specPath := filepath.Join(rootPath, "kindest.yaml")
	spec := fmt.Sprintf(`build:
  name: test/%s
test:
  - name: basic
    env:
      kind: {}
    build:
      name: test/%s-test
      dockerfile: Dockerfile
`, name, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	err := Test(
		&TestOptions{
			File:      specPath,
			Transient: true,
		},
	)
	require.NoError(t, err)
}

func TestTestKindApplyResource(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	script := `#!/bin/bash
set -euo pipefail
namespace=test
kubectl get namespace
if [ -z "$(kubectl get namespace | grep $namespace)" ]; then
	echo "Namespace '$namespace' not found"
	exit 1
fi
echo "Manifests were applied correctly!"`
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
CMD ["/script"]`)
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
      kind:
        resources:
          - test.yaml
    build:
      name: test/%s-test
      dockerfile: Dockerfile
`, name, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	require.NoError(t, Test(
		&TestOptions{
			File:      specPath,
			Transient: true,
		},
	))
}

func TestTestKindErrTestFailure(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := fmt.Sprintf(`FROM alpine:3.11.6
CMD ["sh", "-c", "sleep 5; exit 1"]`)
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
      kind: {}
    build:
      name: test/%s-test
      dockerfile: Dockerfile
`, name, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	err := Test(
		&TestOptions{
			File:      specPath,
			Transient: true,
		},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), ErrTestFailed.Error())
}

func TestTestKindErrManifestNotFound(t *testing.T) {
}
