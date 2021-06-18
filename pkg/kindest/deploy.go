package kindest

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/midcontinentcontrols/kindest/pkg/cluster_management"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"go.uber.org/zap"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/storage/driver"
)

type DeployOptions struct {
	Kind          string   `json:"kind"`
	KubeContext   string   `json:"kubeContext"`
	RestartImages []string `json:"restartImages"`
	Wait          bool     `json:"wait"`
}

func (m *Module) Deploy(options *DeployOptions) (string, error) {
	kubeContext := options.KubeContext
	if options.Kind != "" {
		// Ensure cluster is created
		var err error
		kubeContext, err = cluster_management.CreateCluster(options.Kind, m.log)
		if err != nil {
			return "", err
		}
	}
	if m.Spec.Env.Kubernetes != nil {
		if err := applyManifests(
			kubeContext,
			m.Dir(),
			m.Spec.Env.Kubernetes.Resources,
		); err != nil {
			return "", err
		}
		if err := m.installCharts(kubeContext); err != nil {
			return "", err
		}
		if err := restartDeployments(
			kubeContext,
			options.RestartImages,
		); err != nil {
			return "", err
		}
		if options.Wait {
			if err := m.WaitForReady(kubeContext); err != nil {
				return "", err
			}
		}
	}
	return kubeContext, nil
}

func (m *Module) WaitForReady(kubeContext string) error {
	// TODO: wait for all deployments to be ready
	return nil
}

func restartDeployments(kubeContext string, images []string) error {
	client, _, err := clientForContext(kubeContext)
	if err != nil {
		return err
	}
	ds, err := client.AppsV1().
		Deployments("").
		List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, d := range ds.Items {
		for _, c := range d.Spec.Template.Spec.Containers {
			match := false
			for _, img := range images {
				if img == c.Image {
					match = true
					break
				}
			}
			if match {
				cmd := exec.Command(
					"kubectl",
					"rollout",
					"restart",
					"deployment",
					"--context", kubeContext,
					"-n", d.Namespace,
					d.Name,
				)
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if err := cmd.Run(); err != nil {
					return err
				}
			}
		}
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

func (m *Module) installChart(chart *ChartSpec, kubeContext string) error {
	rootPath := m.Dir()
	chartPath := filepath.Clean(filepath.Join(rootPath, chart.Name))
	log := m.log.With(
		zap.String("releaseName", chart.ReleaseName),
		zap.String("name", chart.Name),
		zap.String("namespace", chart.Namespace),
		zap.String("path", chartPath))
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
	log.Debug("Installing chart")
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
		log.Debug("Installing resolved chart", zap.String("vals", fmt.Sprintf("%#v", vals)))
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
