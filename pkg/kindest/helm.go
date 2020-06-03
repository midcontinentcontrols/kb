package kindest

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	kubefake "helm.sh/helm/v3/pkg/kube/fake"

	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"

	"helm.sh/helm/v3/pkg/cli"
)

func (t *TestSpec) installCharts(
	rootPath string,
	options *TestOptions,
) error {
	for _, chart := range t.Env.Kubernetes.Charts {
		if err := t.installChart(
			chart,
			rootPath,
			options,
		); err != nil {
			return fmt.Errorf("failed to install chart '%s': %v", chart.Name, err)
		}
	}
	return nil
}

func debug(format string, v ...interface{}) {
	log.Debug(fmt.Sprintf(format, v...))
}

// This function loads releases into the memory storage if the
// environment variable is properly set.
func loadReleasesInMemory(actionConfig *action.Configuration, env *cli.EnvSettings) error {
	filePaths := strings.Split(os.Getenv("HELM_MEMORY_DRIVER_DATA"), ":")
	if len(filePaths) == 0 {
		return nil
	}

	store := actionConfig.Releases
	mem, ok := store.Driver.(*driver.Memory)
	if !ok {
		// For an unexpected reason we are not dealing with the memory storage driver.
		return nil
	}

	actionConfig.KubeClient = &kubefake.PrintingKubeClient{Out: ioutil.Discard}

	for _, path := range filePaths {
		b, err := ioutil.ReadFile(path)
		if err != nil {
			return fmt.Errorf("Unable to read memory driver data: %v", err)
		}

		releases := []*release.Release{}
		if err := yaml.Unmarshal(b, &releases); err != nil {
			return fmt.Errorf("Unable to unmarshal memory driver data: %v", err)
		}

		for _, rel := range releases {
			if err := store.Create(rel); err != nil {
				return err
			}
		}
	}

	// Must reset namespace to the proper one
	mem.SetNamespace(env.Namespace())

	return nil
}

func (t *TestSpec) installChart(
	chart *ChartSpec,
	rootPath string,
	options *TestOptions,
) error {
	chartPath := filepath.Clean(filepath.Join(rootPath, chart.Name))
	log := log.With(
		zap.String("releaseName", chart.ReleaseName),
		zap.String("name", chart.Name),
		zap.String("namespace", chart.Namespace),
		zap.String("path", chartPath),
	)
	log.Info("Installing chart")
	env := cli.New()
	cfg := new(action.Configuration)
	helmDriver := os.Getenv("HELM_DRIVER")
	if err := cfg.Init(
		env.RESTClientGetter(),
		env.Namespace(),
		helmDriver,
		debug,
	); err != nil {
		return err
	}
	if helmDriver == "memory" {
		loadReleasesInMemory(cfg, env)
	}
	client := action.NewInstall(cfg)
	cp, err := client.ChartPathOptions.LocateChart(chartPath, env)
	if err != nil {
		return err
	}
	chartRequested, err := loader.Load(cp)
	if err != nil {
		return err
	}
	validInstallableChart, err := isChartInstallable(chartRequested)
	if !validInstallableChart {
		return err
	}
	if chartRequested.Metadata.Deprecated {
		log.Warn("This chart is deprecated")
	}
	if req := chartRequested.Metadata.Dependencies; req != nil {
		return fmt.Errorf("chart dependencies are not yet implemented")
		/*
			// If CheckDependencies returns an error, we have unfulfilled dependencies.
			// As of Helm 2.4.0, this is treated as a stopping condition:
			// https://github.com/helm/helm/issues/2209
			if err := action.CheckDependencies(chartRequested, req); err != nil {
				if client.DependencyUpdate {
					man := &downloader.Manager{
						//Out:              out,
						//ChartPath:        cp,
						//Keyring:          client.ChartPathOptions.Keyring,
						//SkipUpdate:       false,
						//Getters:          p,
						//RepositoryConfig: settings.RepositoryConfig,
						//RepositoryCache:  settings.RepositoryCache,
						//Debug:            settings.Debug,
					}
					if err := man.Update(); err != nil {
						return err
					}
					// Reload the chart with the updated Chart.lock file.
					if chartRequested, err = loader.Load(cp); err != nil {
						return fmt.Errorf("failed reloading chart after repo update: %v", err)
					}
				} else {
					return err
				}
			}*/
	}
	values := chart.Values
	if values == nil {
		values = make(map[string]interface{})
	}

	histClient := action.NewHistory(cfg)
	histClient.Max = 1
	if _, err := histClient.Run(chart.ReleaseName); err == driver.ErrReleaseNotFound {
		// TODO: fix upgrade code
		// https://github.com/helm/helm/blob/master/cmd/helm/upgrade.go
		client.CreateNamespace = true
		client.Replace = true
		// TODO: find out why deployments always end up in default namespace
		client.Namespace = chart.Namespace
		client.ReleaseName = chart.ReleaseName
		log.Info("Installing resolved chart")
		release, err := client.Run(chartRequested, values)
		if err != nil {
			return err
		}
		log.Info("Installed chart", zap.Int("version", release.Version))
	} else {
		return fmt.Errorf("helm upgrade is unimplemented")
	}
	return nil
}
