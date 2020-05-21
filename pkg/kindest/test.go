package kindest

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/docker/docker/api/types"
	containertypes "github.com/docker/docker/api/types/container"
	"github.com/google/uuid"
	"sigs.k8s.io/kind/pkg/cluster"

	"github.com/Jeffail/tunny"
	"github.com/docker/docker/client"
	"github.com/hashicorp/go-multierror"
	"go.uber.org/zap"
)

type TestOptions struct {
	File        string `json:"file,omitempty"`
	Concurrency int    `json:"concurrency,omitempty"`
	Transient   bool   `json:"transient,omitempty"`
}

type TestSpec struct {
	Name  string    `json:"name"`
	Build BuildSpec `json:"build"`
	Env   EnvSpec   `json:"env,omitempty"`
}

var ErrMultipleTestEnv = fmt.Errorf("multiple test environments defined")
var ErrNoTestEnv = fmt.Errorf("no test environment")

func (t *TestSpec) Verify(manifestPath string) error {
	if err := t.Build.Verify(manifestPath); err != nil {
		return err
	}
	if t.Env.Docker != nil {
		if t.Env.Kind != nil {
			return ErrMultipleTestEnv
		}
		return nil
	} else if kind := t.Env.Kind; kind != nil {
		rootDir := filepath.Dir(manifestPath)
		for _, resource := range kind.Resources {
			resourcePath := filepath.Clean(filepath.Join(rootDir, resource))
			if _, err := os.Stat(resourcePath); err != nil {
				return fmt.Errorf("test '%s' env: '%s' not found", t.Name, resourcePath)
			}
		}
		for _, chart := range kind.Charts {
			chartPath := filepath.Join(chart.Path, "Chart.yaml")
			if _, err := os.Stat(chartPath); err != nil {
				return fmt.Errorf("test '%s' env chart '%s': missing Chart.yaml at '%s'", t.Name, chart.ReleaseName, chartPath)
			}
			valuesPath := filepath.Join(chart.Path, "values.yaml")
			if _, err := os.Stat(valuesPath); err != nil {
				return fmt.Errorf("test '%s' env chart '%s': missing values.yaml at '%s'", t.Name, chart.ReleaseName, chartPath)
			}
		}
		return nil
	} else {
		return ErrNoTestEnv
	}
}

func (t *TestSpec) runDocker(
	options *TestOptions,
	cli client.APIClient,
) error {
	var env []string
	for _, v := range t.Env.Variables {
		env = append(env, fmt.Sprintf("%s=%s", v.Name, v.Value))
	}
	resp, err := cli.ContainerCreate(
		context.TODO(),
		&containertypes.Config{
			Image: t.Build.Name + ":latest",
			Env:   env,
		},
		&containertypes.HostConfig{
			AutoRemove: true,
		},
		nil,
		"",
	)
	if err != nil {
		return err
	}
	container := resp.ID
	log := log.With(zap.String("test.Name", t.Name))
	for _, warning := range resp.Warnings {
		log.Debug("Docker", zap.String("warning", warning))
	}
	if err := cli.ContainerStart(
		context.TODO(),
		container,
		types.ContainerStartOptions{},
	); err != nil {
		return err
	}
	logs, err := cli.ContainerLogs(
		context.TODO(),
		container,
		types.ContainerLogsOptions{
			Follow:     true,
			ShowStdout: true,
			ShowStderr: true,
		},
	)
	if err != nil {
		return err
	}
	rd := bufio.NewReader(logs)
	for {
		message, err := rd.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		log.Info(message)
	}
	ch, e := cli.ContainerWait(
		context.TODO(),
		container,
		containertypes.WaitConditionRemoved,
	)
	select {
	case v := <-ch:
		if v.Error != nil {
			return fmt.Errorf(v.Error.Message)
		}
		if v.StatusCode != 0 {
			return fmt.Errorf("exit code %d", v.StatusCode)
		}
		return nil
	case err := <-e:
		return err
	}
}

func clientForCluster(name string, provider *cluster.Provider) (*kubernetes.Clientset, error) {
	kubeConfig, err := provider.KubeConfig(name, false)
	if err != nil {
		return nil, err
	}
	config, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeConfig))
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

func (t *TestSpec) runKind(
	options *TestOptions,
	cli client.APIClient,
) error {
	name := "test-" + uuid.New().String()[:8]
	log := log.With(zap.String("name", name))
	log.Info("Creating cluster", zap.Bool("transient", options.Transient))
	provider := cluster.NewProvider()
	if err := provider.Create(name); err != nil {
		return err
	}
	if options.Transient {
		defer func() {
			log.Info("Deleting transient cluster")
			if err := func() error {
				//timeout := 10 * time.Second
				container := name + "-control-plane"
				if err := cli.ContainerStop(
					context.TODO(),
					container,
					nil,
				); err != nil {
					return err
				}
				if err := cli.ContainerRemove(
					context.TODO(),
					container,
					types.ContainerRemoveOptions{
						Force: true,
					},
				); err != nil {
					return err
				}
				return nil
			}(); err != nil {
				log.Error("Error cleaning up transient cluster", zap.String("message", err.Error()))
			} else {
				log.Info("Deleted transient cluster")
			}
		}()
	}
	client, err := clientForCluster(name, provider)
	if err != nil {
		return err
	}

	// TODO: create test pod

	log.Info("Listing pods")
	pods, err := client.CoreV1().Pods("kube-system").List(
		context.TODO(),
		metav1.ListOptions{},
	)
	if err != nil {
		return err
	}
	log.Info("Listed pods", zap.Int("count", len(pods.Items)))
	for _, pod := range pods.Items {
		log.Info("Found pod", zap.String("name", pod.Name))
	}
	return nil
}

func (t *TestSpec) Run(
	manifestPath string,
	options *TestOptions,
	cli client.APIClient,
) error {
	if err := t.Build.Build(
		manifestPath,
		&BuildOptions{
			Concurrency: 1,
		},
		cli,
		nil,
	); err != nil {
		return err
	}
	if t.Env.Docker != nil {
		return t.runDocker(options, cli)
	} else if t.Env.Kind != nil {
		return t.runKind(options, cli)
	} else {
		panic("unreachable branch detected")
	}
}

var ErrNoTests = fmt.Errorf("no tests configured")

type testRun struct {
	test         *TestSpec
	options      *TestOptions
	manifestPath string
}

func Test(options *TestOptions) error {
	cli, err := client.NewEnvClient()
	if err != nil {
		return err
	}
	var pool *tunny.Pool
	concurrency := options.Concurrency
	if concurrency == 0 {
		concurrency = runtime.NumCPU()
	}
	pool = tunny.NewFunc(concurrency, func(payload interface{}) interface{} {
		item := payload.(*testRun)
		return item.test.Run(item.manifestPath, item.options, cli)
	})
	defer pool.Close()
	return TestEx(options, pool)
}

func TestEx(options *TestOptions, pool *tunny.Pool) error {
	spec, manifestPath, err := loadSpec(options.File)
	if err != nil {
		return err
	}
	log.Info("Loaded spec", zap.String("path", manifestPath))
	if len(spec.Test) == 0 {
		return ErrNoTests
	}
	n := len(spec.Test)
	dones := make([]chan error, n, n)
	for i, test := range spec.Test {
		done := make(chan error, 1)
		dones[i] = done
		go func(test *TestSpec, done chan<- error) {
			defer close(done)
			err, _ := pool.Process(&testRun{
				manifestPath: manifestPath,
				test:         test,
				options:      options,
			}).(error)
			done <- err
		}(test, done)
	}
	var multi error
	for i, done := range dones {
		if err := <-done; err != nil {
			multi = multierror.Append(multi, fmt.Errorf("%s: %v", spec.Test[i].Name, err))
		}
	}
	return multi
}
