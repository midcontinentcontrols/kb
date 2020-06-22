package kindest

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/midcontinentcontrols/kindest/pkg/logger"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/kind/pkg/cluster"
)

func newTestLogger() logger.Logger {
	return logger.NewZapLoggerFromEnv()
}

type testEnv struct {
	files map[string]interface{}
}

func createTestEnv(t *testing.T, files map[string]interface{}) {

}

func TestNoTests(t *testing.T) {
	specPath := createBasicTestProject(t, "tmp")
	defer os.RemoveAll(filepath.Dir(filepath.Dir(specPath)))
	require.Equal(t, ErrNoTests, Test(
		&TestOptions{
			File:       specPath,
			NoRegistry: true,
		},
		newTestLogger(),
	))
}

func createFiles(files map[string]interface{}, dir string) error {
	if err := os.MkdirAll(dir, 0766); err != nil {
		return err
	}
	for k, v := range files {
		path := filepath.Join(dir, k)
		if m, ok := v.(map[string]interface{}); ok {
			if err := createFiles(m, path); err != nil {
				return fmt.Errorf("%s: %v", path, err)
			}
		} else if s, ok := v.(string); ok {
			if err := ioutil.WriteFile(
				path,
				[]byte(s),
				0644,
			); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("unknown type for %s", path)
		}
	}
	return nil
}

func runTest(
	t *testing.T,
	files func(name string) map[string]interface{},
	f func(t *testing.T, rootPath string),
) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, createFiles(files(name), rootPath))
	f(t, rootPath)
}

func TestTestDockerEnv(t *testing.T) {
	files := func(name string) map[string]interface{} {
		return map[string]interface{}{
			"Dockerfile": `FROM alpine:latest
CMD ["sh", "-c", "set -eu; echo $MESSAGE"]`,
			"kindest.yaml": fmt.Sprintf(`build:
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
`, name),
		}
	}
	runTest(
		t,
		files,
		func(t *testing.T, rootPath string) {
			require.NoError(t, Test(
				&TestOptions{
					File:       filepath.Join(rootPath, "kindest.yaml"),
					NoRegistry: true,
				},
				newTestLogger(),
			))
		},
	)
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
			File:       specPath,
			NoRegistry: true,
		},
		newTestLogger(),
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
      kubernetes: {}
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
			File:       specPath,
			NoRegistry: true,
		},
		newTestLogger(),
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
			File:   specPath,
			NoPush: true,
		},
		newTestLogger(),
	)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "exit code 1"))
}

func TestTestKubernetesTransientKind(t *testing.T) {
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
      kubernetes: {}
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
			File:       specPath,
			NoRegistry: true,
			Transient:  true,
		},
		newTestLogger(),
	)
	require.NoError(t, err)
}

func TestTestKubernetesApplyResource(t *testing.T) {
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
	exit 2
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
      kubernetes:
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
			File:       specPath,
			NoRegistry: true,
			Transient:  true,
		},
		newTestLogger(),
	))
}

func TestTestKubernetesErrTestFailure(t *testing.T) {
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
      kubernetes: {}
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
			File:       specPath,
			NoRegistry: true,
			Transient:  true,
		},
		logger.NewZapLoggerFromEnv(),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), ErrTestFailed.Error())
}

func TestTestKubernetesErrManifestNotFound(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := fmt.Sprintf(`FROM alpine:3.11.6
CMD ["sh", "-c", "sleep 5; echo 'All done!'"]`)
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
      kubernetes:
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
	err := Test(
		&TestOptions{
			File:       specPath,
			NoRegistry: true,
			Transient:  true,
		},
		newTestLogger(),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "test.yaml' not found")
}

func TestTestKubernetesErrUnknownCluster(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := fmt.Sprintf(`FROM alpine:3.11.6
CMD ["sh", "-c", "sleep 5; echo 'All done!'"]`)
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
      kubernetes: {}
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
			File:       specPath,
			NoRegistry: true,
		},
		newTestLogger(),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), ErrUnknownCluster.Error())
}

func TestTestKubernetesErrContextNotFound(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := fmt.Sprintf(`FROM alpine:3.11.6
CMD ["sh", "-c", "sleep 5; echo 'All done!'"]`)
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
      kubernetes: {}
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
			File:       specPath,
			Context:    name,
			NoRegistry: true,
		},
		newTestLogger(),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), fmt.Sprintf("context \"%s\" does not exist", name))
}

func TestTestKubernetesCreateKind(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	provider := cluster.NewProvider()
	defer func() {
		clusters, err := provider.List()
		require.NoError(t, err)
		found := false
		for _, cluster := range clusters {
			if cluster == name {
				found = true
				break
			}
		}
		require.True(t, found)
		require.NoError(t, provider.Delete(name, ""))
	}()
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
      kubernetes: {}
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
			File:       specPath,
			Kind:       name,
			NoRegistry: true,
		},
		newTestLogger(),
	)
	require.NoError(t, err)
}

func TestTestKubernetesExistingKind(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	provider := cluster.NewProvider()
	require.NoError(t, provider.Create(name))
	defer func() {
		clusters, err := provider.List()
		require.NoError(t, err)
		found := false
		for _, cluster := range clusters {
			if cluster == name {
				found = true
				break
			}
		}
		require.True(t, found)
		require.NoError(t, provider.Delete(name, ""))
	}()
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
      kubernetes: {}
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
			File:       specPath,
			Kind:       name,
			NoRegistry: true,
		},
		newTestLogger(),
	)
	require.NoError(t, err)
}

func TestTestKindMultipleUses(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := fmt.Sprintf(`FROM alpine:3.11.6
CMD ["sh", "-c", "echo \"Hello world!\""]`)
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
      kubernetes: {}
    build:
      name: test/%s-test
      dockerfile: Dockerfile
`, name, name)
	require.NoError(t, ioutil.WriteFile(
		specPath,
		[]byte(spec),
		0644,
	))
	kindName := "test-" + uuid.New().String()[:8]
	require.NoError(t, Test(
		&TestOptions{
			File:       specPath,
			Kind:       kindName,
			Transient:  false,
			NoRegistry: false,
		},
		newTestLogger(),
	))
	defer func() {
		provider := cluster.NewProvider()
		require.NoError(t, provider.Delete(kindName, ""))
	}()
	require.NoError(t, Test(
		&TestOptions{
			File:       specPath,
			Kind:       kindName,
			Transient:  false,
			NoRegistry: false,
		},
		newTestLogger(),
	))
	require.NoError(t, Test(
		&TestOptions{
			File:       specPath,
			Kind:       kindName,
			Transient:  false,
			NoRegistry: false,
		},
		newTestLogger(),
	))
}
