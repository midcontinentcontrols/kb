package kindest

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"github.com/midcontinentcontrols/kindest/pkg/test"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stretchr/testify/require"
)

//
//
func TestModuleBuildKaniko(t *testing.T) {
	//
	//
	t.Run("BuildStatusSucceeded", func(t *testing.T) {
		test.WithTemporaryModule(t, func(name, rootPath string) {
			specYaml := fmt.Sprintf(`build:
  name: %s`, name)
			dockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "echo \"Hello, world\""]`
			require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
				"kindest.yaml": specYaml,
				"Dockerfile":   dockerfile,
			}))
			log := logger.NewMockLogger(logger.NewFakeLogger())
			module, err := NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
			require.NoError(t, err)
			require.Equal(t, BuildStatusPending, module.Status())
			test.WithTemporaryCluster(t, name, log, func(kubeContext string, cl client.Client) {
				require.NoError(t, module.Build(&BuildOptions{
					Builder: "kaniko",
					Context: kubeContext,
					NoPush:  true,
				}))
				require.Equal(t, BuildStatusSucceeded, module.Status())
			})
		})
	})

	//
	//
	t.Run("BuildStatusFailed", func(t *testing.T) {
		test.WithTemporaryModule(t, func(name, rootPath string) {
			specYaml := fmt.Sprintf(`build:
  name: %s`, name)
			dockerfile := `FROM alpine:3.11.6
RUN cat foo`
			require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
				"kindest.yaml": specYaml,
				"Dockerfile":   dockerfile,
			}))
			log := logger.NewMockLogger(logger.NewFakeLogger())
			p := NewProcess(runtime.NumCPU(), log)
			module, err := p.GetModule(filepath.Join(rootPath, "kindest.yaml"))
			require.NoError(t, err)
			test.WithTemporaryCluster(t, name, log, func(kubeContext string, cl client.Client) {
				err := module.Build(&BuildOptions{
					Builder: "kaniko",
					Context: kubeContext,
					NoPush:  true,
				})
				require.Error(t, err)
				require.Contains(t, err.Error(), "kaniko: command terminated with exit code 1")
				require.Equal(t, BuildStatusFailed, module.Status())
			})
		})
	})

	t.Run("PrivateImagePushPull", func(t *testing.T) {
		test.WithTemporaryModule(t, func(name string, rootPath string) {
			pushImage := test.GetPushImage()
			depYaml := fmt.Sprintf(`build:
  name: %s-dep`, pushImage)
			specYaml := fmt.Sprintf(`dependencies:
- dep
build:
  name: %s`, pushImage)
			depDockerfile := `FROM alpine:3.11.6
CMD ["sh", "-c", "echo \"Hello, world\""]`
			dockerfile := fmt.Sprintf(`FROM %s-dep:latest
CMD ["sh", "-c", "echo \"foo bar baz\""]`, pushImage)
			require.NoError(t, test.CreateFiles(rootPath, map[string]interface{}{
				"kindest.yaml": specYaml,
				"Dockerfile":   dockerfile,
				"dep": map[string]interface{}{
					"kindest.yaml": depYaml,
					"Dockerfile":   depDockerfile,
				},
			}))
			log := logger.NewMockLogger(logger.NewFakeLogger())
			module, err := NewProcess(runtime.NumCPU(), log).GetModule(filepath.Join(rootPath, "kindest.yaml"))
			require.NoError(t, err)
			require.Equal(t, BuildStatusPending, module.Status())
			test.WithTemporaryCluster(t, name, log, func(kubeContext string, cl client.Client) {
				require.NoError(t, module.Build(&BuildOptions{
					Builder: "kaniko",
					Context: kubeContext,
					Verbose: true,
				}))
				require.Equal(t, BuildStatusSucceeded, module.Status())
			})
		})
	})
}
