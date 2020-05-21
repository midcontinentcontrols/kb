package kindest

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/docker/docker/api/types"
	containertypes "github.com/docker/docker/api/types/container"

	"github.com/Jeffail/tunny"
	"github.com/docker/docker/client"
	"github.com/hashicorp/go-multierror"
	"go.uber.org/zap"
)

type TestOptions struct {
	File        string `json:"file,omitempty"`
	Concurrency int    `json:"concurrency,omitempty"`
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
	} else {
		return ErrNoTestEnv
	}
	return nil
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
		return fmt.Errorf("unimplemented")
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
