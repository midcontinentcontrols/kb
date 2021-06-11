package kindest

import (
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/storage/driver"
)

type DeployOptions struct {
	KubeContext string `json:"kubeContext"`
}

func (m *Module) Deploy(options *DeployOptions) error {
	if err := applyManifests(
		options.KubeContext,
		m.Dir(),
		m.Spec.Env.Kubernetes.Resources,
	); err != nil {
		return err
	}
	if err := m.installCharts(options.KubeContext); err != nil {
		return err
	}
	return nil
}

func (m *Module) installCharts(kubeContext string) error {
	for _, chart := range m.Spec.Env.Kubernetes.Charts {
		if err := m.installChart(
			chart,
			kubeContext,
		); err != nil {
			return fmt.Errorf("failed to install chart '%s': %v", chart.Name, err)
		}
	}
	return nil
}

func (m *Module) installChart(
	chart *ChartSpec,
	kubeContext string,
) error {
	rootPath := m.Dir()
	chartPath := filepath.Clean(filepath.Join(rootPath, chart.Name))
	log := m.log.With(
		zap.String("releaseName", chart.ReleaseName),
		zap.String("name", chart.Name),
		zap.String("namespace", chart.Namespace),
		zap.String("path", chartPath),
	)
	env := createHelmEnv(chart.Namespace, kubeContext)
	cfg := new(action.Configuration)
	helmDriver := os.Getenv("HELM_DRIVER")
	if err := cfg.Init(
		env.RESTClientGetter(),
		chart.Namespace,
		helmDriver,
		debug(log),
	); err != nil {
		return err
	}
	if helmDriver == "memory" {
		loadReleasesInMemory(cfg, env, chart.Namespace)
	}

	valueOpts := &values.Options{
		ValueFiles: chart.ValuesFiles,
	}
	vals, err := valueOpts.MergeValues(getter.All(env))
	if err != nil {
		return err
	}
	explicit, err := ensureMapKeysAreStrings(chart.Values)
	if err != nil {
		return err
	}
	vals = mergeMaps(vals, explicit)

	log.Info("Installing chart")
	//zap.String("vals", fmt.Sprintf("%#v", vals)), zap.String("original", fmt.Sprintf("%#v", chart.Values)))

	histClient := action.NewHistory(cfg)
	histClient.Max = 1
	if _, err := histClient.Run(chart.ReleaseName); err == driver.ErrReleaseNotFound {
		install := action.NewInstall(cfg)
		cp, err := install.ChartPathOptions.LocateChart(chartPath, env)
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
		install.CreateNamespace = true
		//install.Replace = true
		install.Namespace = chart.Namespace
		install.ReleaseName = chart.ReleaseName
		log.Info("Installing resolved chart", zap.String("vals", fmt.Sprintf("%#v", vals)))
		release, err := install.Run(chartRequested, vals)
		if err != nil {
			return err
		}
		log.Info("Installed chart", zap.Int("version", release.Version))
	} else if err != nil {
		return err
	}
	upgrade := action.NewUpgrade(cfg)
	cp, err := upgrade.ChartPathOptions.LocateChart(chartPath, env)
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
	rel, err := upgrade.Run(chart.ReleaseName, chartRequested, vals)
	if err != nil {
		return err
	}
	log.Info("Chart upgraded", zap.Int("version", rel.Version))
	return nil
}
