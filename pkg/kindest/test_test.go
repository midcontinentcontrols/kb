package kindest

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/midcontinentcontrols/kindest/pkg/logger"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestTestDockerEnv(t *testing.T) {
	name := "test-" + uuid.New().String()[:8]
	rootPath := filepath.Join("tmp", name)
	require.NoError(t, os.MkdirAll(rootPath, 0766))
	defer os.RemoveAll(rootPath)
	dockerfile := `FROM alpine:latest
CMD ["sh", "-c", "set -euo pipefail; echo $MYVARIABLE"]`
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
        - name: MYVARIABLE
          value: foobarbaz
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
	log := logger.NewMockLogger(logger.NewFakeLogger())
	p := NewProcess(runtime.NumCPU(), log)
	module, err := p.GetModule(specPath)
	require.NoError(t, err)
	require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
	test, err := p.GetModuleFromBuildSpec(specPath, &module.Spec.Test[0].Build)
	require.NoError(t, err)
	require.NoError(t, test.Build(&BuildOptions{NoPush: true}))
	err = module.Spec.RunTests(&TestOptions{}, log)
	require.NoError(t, err)
}

func TestTestErrNoEnv(t *testing.T) {
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
	log := logger.NewMockLogger(logger.NewFakeLogger())
	p := NewProcess(runtime.NumCPU(), log)
	_, err := p.GetModule(specPath)
	require.Equal(t, ErrNoTestEnv, err)
}

func TestTestErrMultipleEnv(t *testing.T) {
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
	log := logger.NewMockLogger(logger.NewFakeLogger())
	p := NewProcess(runtime.NumCPU(), log)
	_, err := p.GetModule(specPath)
	require.Equal(t, ErrMultipleTestEnv, err)
}

func TestTestErrorDocker(t *testing.T) {
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
	log := logger.NewMockLogger(logger.NewFakeLogger())
	p := NewProcess(runtime.NumCPU(), log)
	module, err := p.GetModule(specPath)
	require.NoError(t, err)
	require.NoError(t, module.Build(&BuildOptions{NoPush: true}))
	test, err := p.GetModuleFromBuildSpec(specPath, &module.Spec.Test[0].Build)
	require.NoError(t, err)
	require.NoError(t, test.Build(&BuildOptions{NoPush: true}))
	err = module.Spec.RunTests(&TestOptions{}, log)
	require.Error(t, err)
	require.Truef(t, strings.Contains(err.Error(), "exit code 1"), "got error '%s'", err.Error())
}

/*
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
		test.NewTestLogger(),
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
		test.NewTestLogger(),
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
		test.NewTestLogger(),
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
		test.NewTestLogger(),
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
		test.NewTestLogger(),
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
		test.NewTestLogger(),
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
		test.NewTestLogger(),
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
		test.NewTestLogger(),
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
		test.NewTestLogger(),
	))
	require.NoError(t, Test(
		&TestOptions{
			File:       specPath,
			Kind:       kindName,
			Transient:  false,
			NoRegistry: false,
		},
		test.NewTestLogger(),
	))
}

*/
