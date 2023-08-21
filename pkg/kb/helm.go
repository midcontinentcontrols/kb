package kb

import (
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"sync"

	"github.com/midcontinentcontrols/kb/pkg/logger"
	"gopkg.in/yaml.v2"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	kubefake "helm.sh/helm/v3/pkg/kube/fake"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"

	"helm.sh/helm/v3/pkg/cli"
)

func ensureMapKeysAreStrings(m map[interface{}]interface{}) (map[string]interface{}, error) {
	r := map[string]interface{}{}
	for k, v := range m {
		typedKey, ok := k.(string)
		if !ok {
			return nil, fmt.Errorf("unexpected type %s for key %s", reflect.TypeOf(k), k)
		}
		switch v.(type) {
		case []interface{}:
			va := v.([]interface{})
			for i, item := range va {
				if vi, ok := item.(map[interface{}]interface{}); ok {
					var err error
					va[i], err = ensureMapKeysAreStrings(vi)
					if err != nil {
						return nil, err
					}
				}
			}
			r[typedKey] = va
		case map[interface{}]interface{}:
			nestedMapWithStringKeys, err := ensureMapKeysAreStrings(v.(map[interface{}]interface{}))
			if err != nil {
				return nil, err
			}
			r[typedKey] = nestedMapWithStringKeys
		default:
			r[typedKey] = v
		}
	}
	return r, nil
}

/*
func (t *TestSpec) installCharts(
	rootPath string,
	kubeContext string,
	//options *TestOptions,
	log logger.Logger,
) error {
	for _, chart := range t.Env.Kubernetes.Charts {
		if err := t.installChart(
			chart,
			rootPath,
			kubeContext,
			//options,
			log,
		); err != nil {
			return fmt.Errorf("failed to install chart '%s': %v", chart.Name, err)
		}
	}
	return nil
}*/

func debug(log logger.Logger) func(string, ...interface{}) {
	return func(format string, v ...interface{}) {
		log.Debug(fmt.Sprintf(format, v...))
	}
}

// This function loads releases into the memory storage if the
// environment variable is properly set.
func loadReleasesInMemory(actionConfig *action.Configuration, env *cli.EnvSettings, namespace string) error {
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
	mem.SetNamespace(namespace)

	return nil
}

// isChartInstallable validates if a chart can be installed
// Application chart type is only installable
func isChartInstallable(ch *chart.Chart) (bool, error) {
	switch ch.Metadata.Type {
	case "", "application":
		return true, nil
	}
	return false, fmt.Errorf("%s charts are not installable", ch.Metadata.Type)
}

var helmL sync.Mutex

func createHelmEnv(namespace string, kubeContext string) *cli.EnvSettings {
	helmL.Lock()
	defer helmL.Unlock()
	cv := os.Getenv("HELM_NAMESPACE")
	kc := os.Getenv("HELM_NAMESPACE")
	os.Setenv("HELM_NAMESPACE", namespace)
	os.Setenv("HELM_KUBECONTEXT", kubeContext)
	env := cli.New()
	os.Setenv("HELM_NAMESPACE", cv)
	os.Setenv("HELM_KUBECONTEXT", kc)
	return env
}

func mergeMaps(a, b map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(a))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if v, ok := v.(map[string]interface{}); ok {
			if bv, ok := out[k]; ok {
				if bv, ok := bv.(map[string]interface{}); ok {
					out[k] = mergeMaps(bv, v)
					continue
				}
			}
		}
		out[k] = v
	}
	return out
}

/*
func (t *TestSpec) installChart(
	chart *ChartSpec,
	rootPath string,
	kubeContext string,
	//options *TestOptions,
	log logger.Logger,
) error {
	chartPath := filepath.Clean(filepath.Join(rootPath, chart.Name))
	log = log.With(
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
*/
