package kindest

import (
	"fmt"
	"runtime"

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
	Env   *EnvSpec  `json:"env,omitempty"`
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
	if t.Env != nil {
		for _, variable := range t.Env.Variables {
		}
	}
	return nil
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
