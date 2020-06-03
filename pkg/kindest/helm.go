package kindest

import (
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"

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
	client.Namespace = chart.Namespace
	release, err := client.Run(chartRequested, values)
	if err != nil {
		return err
	}
	log.Info("Installed chart", zap.Int("version", release.Version))
	return nil
}
