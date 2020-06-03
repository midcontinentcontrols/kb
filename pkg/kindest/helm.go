package kindest

import (
	"path/filepath"

	"go.uber.org/zap"
	"helm.sh/helm/v3/pkg/action"
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
			return err
		}
	}
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
	cfg := &action.Configuration{}
	client := action.NewInstall(cfg)
	env := cli.New()
	_, err := client.ChartPathOptions.LocateChart(chartPath, env)
	if err != nil {
		return err
	}
	/*
		chartRequested, err := loader.Load(cp)
		if err != nil {
			return err
		}
		valueOpts := &values.Options{}
		validInstallableChart, err := isChartInstallable(chartRequested)
		if !validInstallableChart {
			return nil, err
		}
		if chartRequested.Metadata.Deprecated {
			log.Warn("Chart is deprecated", zap.String("name", chart))
		}
		if req := chartRequested.Metadata.Dependencies; req != nil {
			// If CheckDependencies returns an error, we have unfulfilled dependencies.
			// As of Helm 2.4.0, this is treated as a stopping condition:
			// https://github.com/helm/helm/issues/2209
			if err := action.CheckDependencies(chartRequested, req); err != nil {
				if client.DependencyUpdate {
					man := &downloader.Manager{
						Out:              out,
						ChartPath:        cp,
						Keyring:          client.ChartPathOptions.Keyring,
						SkipUpdate:       false,
						Getters:          p,
						RepositoryConfig: settings.RepositoryConfig,
						RepositoryCache:  settings.RepositoryCache,
						Debug:            settings.Debug,
					}
					if err := man.Update(); err != nil {
						return nil, err
					}
					// Reload the chart with the updated Chart.lock file.
					if chartRequested, err = loader.Load(cp); err != nil {
						return nil, errors.Wrap(err, "failed reloading chart after repo update")
					}
				} else {
					return nil, err
				}
			}
		}
		return client.Run(chartRequested, valueOpts)
	*/
	return nil
}
