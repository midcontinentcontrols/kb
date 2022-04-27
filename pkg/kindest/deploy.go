package kindest

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/midcontinentcontrols/kindest/pkg/cluster_management"
	"github.com/midcontinentcontrols/kindest/pkg/logger"
	"github.com/midcontinentcontrols/kindest/pkg/util"
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
	Repository    string   `json:"repository"`
	Tag           string   `json:"tag"`
	NoAutoRestart bool     `json:"noAutoRestart"`
	RestartImages []string `json:"restartImages"`
	Wait          bool     `json:"wait"`
	Verbose       bool     `json:"verbose,omitempty"`
	Force         bool     `json:"force,omitempty"`
}

func (m *Module) HasEnvironment() bool {
	return m.Spec.Env.Docker != nil || m.Spec.Env.Kubernetes != nil
}

var ErrNoEnvironment = fmt.Errorf("no environment in spec")

func (m *Module) Deploy(options *DeployOptions) (string, error) {
	if !m.HasEnvironment() {
		return "", ErrNoEnvironment
	}
	kubeContext := options.KubeContext
	if options.Kind != "" {
		var err error
		kubeContext, err = cluster_management.CreateCluster(options.Kind, m.log)
		if err != nil {
			return "", err
		}
	}
	if err := m.UpdateResources(kubeContext, options.Verbose, options.Force); err != nil {
		return "", err
	}
	if !options.NoAutoRestart {
		if err := m.RestartContainers(
			options.RestartImages,
			options.Verbose,
			kubeContext,
		); err != nil {
			return "", err
		}
	}
	if options.Wait {
		if err := m.WaitForReady(
			kubeContext,
			options.Repository,
			options.Tag,
		); err != nil {
			return "", err
		}
	}
	return kubeContext, nil
}

func (m *Module) UpdateResources(kubeContext string, verbose, force bool) error {
	log := m.log.With(zap.String("kubeContext", kubeContext))
	log.Debug("Updating resources")
	if m.Spec.Env.Docker != nil {
		panic("unimplemented")
	} else if m.Spec.Env.Kubernetes != nil {
		log.Debug("Applying kubernetes manifests")
		if err := m.applyManifests(
			kubeContext,
			m.Dir(),
			m.Spec.Env.Kubernetes.Resources,
			verbose,
			force,
		); err != nil {
			return err
		}
		log.Debug("Kubernetes manifests applied successfully")
		log.Debug("Installing Helm charts")
		if err := m.installCharts(kubeContext, force); err != nil {
			return err
		}
		log.Debug("Helm charts installed")
	} else {
		panic("unreachable branch detected")
	}
	log.Debug("Resources updated successfully")
	return nil
}

func (m *Module) RestartContainers(restartImages []string, verbose bool, kubeContext string) error {
	if m.Spec.Env.Docker != nil {
		panic("unimplemented")
	} else if m.Spec.Env.Kubernetes != nil {
		// TODO: restart all deployment that mount a modified configmap or secret
		if err := restartDeployments(kubeContext, restartImages, verbose, m.log); err != nil {
			return err
		}
		if err := restartDaemonSets(kubeContext, restartImages, verbose, m.log); err != nil {
			return err
		}
		if err := restartStatefulSets(kubeContext, restartImages, verbose, m.log); err != nil {
			return err
		}
	} else {
		panic("unreachable branch detected")
	}
	return nil
}

func (m *Module) waitForReadyPods(kubeContext string, images []string) error {
	cl, err := util.CreateKubeClient(kubeContext)
	if err != nil {
		return err
	}
	if err := waitForDeployments(cl, images, m.log); err != nil {
		return err
	}
	if err := waitForStatefulSets(cl, images, m.log); err != nil {
		return err
	}
	if err := waitForDaemonSets(cl, images, m.log); err != nil {
		return err
	}
	return nil
}

func waitForDeployments(cl client.Client, images []string, log logger.Logger) error {
	deployments := &appsv1.DeploymentList{}
	if err := cl.List(context.TODO(), deployments); err != nil {
		return err
	}
	var wait []*appsv1.Deployment
	for _, deployment := range deployments.Items {
		for _, image := range images {
			found := false
			for _, container := range deployment.Spec.Template.Spec.Containers {
				if container.Image == image {
					wait = append(wait, deployment.DeepCopy())
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}
	log.Debug("Waiting on all Deployments to be Running",
		zap.Int("count", len(wait)),
		zap.Int("numRestartImages", len(images)))
	for _, deployment := range wait {
		log.Debug("Waiting on Deployment",
			zap.String("name", deployment.Name),
			zap.String("namespace", deployment.Namespace))
		if err := util.WaitForDeployment(
			cl,
			deployment.Name,
			deployment.Namespace,
			log,
		); err != nil {
			return fmt.Errorf("error waiting for Deployment %s/%s: %v", deployment.Namespace, deployment.Name, err)
		}
	}
	return nil
}

func waitForStatefulSets(cl client.Client, images []string, log logger.Logger) error {
	statefulSets := &appsv1.StatefulSetList{}
	if err := cl.List(context.TODO(), statefulSets); err != nil {
		return err
	}
	var wait []*appsv1.StatefulSet
	for _, statefulSet := range statefulSets.Items {
		for _, image := range images {
			found := false
			for _, container := range statefulSet.Spec.Template.Spec.Containers {
				if container.Image == image {
					wait = append(wait, statefulSet.DeepCopy())
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}
	log.Debug("Waiting on all StatefulSets to be Running",
		zap.Int("count", len(wait)),
		zap.Int("numRestartImages", len(images)))
	for _, statefulSet := range wait {
		log.Info("Waiting on StatefulSet",
			zap.String("name", statefulSet.Name),
			zap.String("namespace", statefulSet.Namespace))
		if err := util.WaitForStatefulSet(
			cl,
			statefulSet.Name,
			statefulSet.Namespace,
			log,
		); err != nil {
			return fmt.Errorf("error waiting for StatefulSet %s/%s: %v", statefulSet.Namespace, statefulSet.Name, err)
		}
	}
	return nil
}

func waitForDaemonSets(cl client.Client, images []string, log logger.Logger) error {
	daemonSets := &appsv1.DaemonSetList{}
	if err := cl.List(context.TODO(), daemonSets); err != nil {
		return err
	}
	var wait []*appsv1.DaemonSet
	for _, daemonSet := range daemonSets.Items {
		for _, image := range images {
			found := false
			for _, container := range daemonSet.Spec.Template.Spec.Containers {
				if container.Image == image {
					wait = append(wait, daemonSet.DeepCopy())
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}
	log.Debug("Waiting on all DaemonSets to be Running",
		zap.Int("count", len(wait)),
		zap.Int("numRestartImages", len(images)))
	for _, daemonSet := range wait {
		log.Info("Waiting on DaemonSet",
			zap.String("name", daemonSet.Name),
			zap.String("namespace", daemonSet.Namespace))
		if err := util.WaitForDaemonSet(
			cl,
			daemonSet.Name,
			daemonSet.Namespace,
			log,
		); err != nil {
			return fmt.Errorf("error waiting for DaemonSet %s/%s: %v", daemonSet.Namespace, daemonSet.Name, err)
		}
	}
	return nil
}

func (m *Module) WaitForReady(kubeContext, repository, tag string) error {
	// TODO: inspect all charts and get deployments that aren't running images built by a module
	images, err := m.ListImages()
	if err != nil {
		return fmt.Errorf("ListImages: %v", err)
	}
	for i, image := range images {
		images[i] = util.SanitizeImageName(repository, image, tag)
	}
	if m.Spec.Env.Docker != nil {
		panic("unimplemented")
	} else if m.Spec.Env.Kubernetes != nil {
		return m.waitForReadyPods(kubeContext, images)
	} else {
		panic("unreachable branch detected")
	}
	return nil
}

func restartDeployments(kubeContext string, images []string, verbose bool, log logger.Logger) error {
	client, _, err := util.ClientsetForContext(kubeContext)
	if err != nil {
		return err
	}
	ds, err := client.AppsV1().
		Deployments("").
		List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	log.Debug("Checking Deployments to restart", zap.Int("numDeployments", len(ds.Items)))
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
				log.Info("Restarting Deployment",
					zap.String("name", d.Name),
					zap.String("namespace", d.Namespace))
				cmd := exec.Command(
					"kubectl",
					"rollout",
					"restart",
					"deployment",
					"--context", kubeContext,
					"-n", d.Namespace,
					d.Name,
				)
				if verbose {
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
				}
				if err := cmd.Run(); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func restartStatefulSets(kubeContext string, images []string, verbose bool, log logger.Logger) error {
	client, _, err := util.ClientsetForContext(kubeContext)
	if err != nil {
		return err
	}
	ds, err := client.AppsV1().
		StatefulSets("").
		List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	log.Debug("Checking StatefulSets to restart", zap.Int("numStatefulSets", len(ds.Items)))
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
				log.Debug("Restarting StatefulSet",
					zap.String("name", d.Name),
					zap.String("namespace", d.Namespace))
				cmd := exec.Command(
					"kubectl",
					"rollout",
					"restart",
					"statefulset",
					"--context", kubeContext,
					"-n", d.Namespace,
					d.Name,
				)
				if verbose {
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
				}
				if err := cmd.Run(); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func restartDaemonSets(kubeContext string, images []string, verbose bool, log logger.Logger) error {
	client, _, err := util.ClientsetForContext(kubeContext)
	if err != nil {
		return err
	}
	ds, err := client.AppsV1().
		DaemonSets("").
		List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	log.Debug("Checking DaemonSets to restart", zap.Int("numDaemonSets", len(ds.Items)))
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
				log.Debug("Restarting DaemonSets",
					zap.String("name", d.Name),
					zap.String("namespace", d.Namespace))
				cmd := exec.Command(
					"kubectl",
					"rollout",
					"restart",
					"daemonset",
					"--context", kubeContext,
					"-n", d.Namespace,
					d.Name,
				)
				if verbose {
					cmd.Stdout = os.Stdout
					cmd.Stderr = os.Stderr
				}
				if err := cmd.Run(); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func digestChart(name string, chart *ChartSpec) (string, error) {
	h := md5.New()
	h.Write([]byte("chart?" + name))
	body, err := yaml.Marshal(chart)
	if err != nil {
		return "", fmt.Errorf("yaml.Marshal: %v", err)
	}
	if _, err := h.Write(body); err != nil {
		return "", err
	}
	if _, err := hashPath(chart.Name, h); err != nil {
		return "", fmt.Errorf("hashPath: %v", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (m *Module) installCharts(kubeContext string, force bool) error {
	for name, chart := range m.Spec.Env.Kubernetes.Charts {
		newDigest, err := digestChart(name, chart)
		if err != nil {
			return fmt.Errorf("digestChart: %v", err)
		}
		key := "chart?" + name
		if !force {
			oldDigest, err := m.CachedDigest(key)
			if err != nil && err != ErrModuleNotCached {
				return fmt.Errorf("CachedDigest: %v", err)
			}
			if newDigest == oldDigest {
				m.log.Debug("No changes for chart",
					zap.String("name", name),
					zap.String("digest", oldDigest))
				continue
			}
		}
		if err := m.installChart(
			chart,
			kubeContext,
		); err != nil {
			return fmt.Errorf("failed to install chart '%s': %v", chart.Name, err)
		}
		if err := m.cacheDigest(key, newDigest); err != nil {
			return fmt.Errorf("cacheDigest: %v", err)
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
	log.Debug("Chart upgraded", zap.Int("version", rel.Version))
	return nil
}
