package kindest

type DeployOptions struct {
	KubeContext string `json:"kubeContext"`
}

func (m *Module) Deploy(options *DeployOptions) error {
	// TODO: move to deploy phase
	//if err := applyTestManifests(
	//	options.KubeContext,
	//	rootPath,
	//	t.Env.Kubernetes.Resources,
	//); err != nil {
	//	return err
	//}
	//if err := t.installCharts(
	//	m.Path,
	//	options.KubeContext,
	//	options,
	//	log,
	//); err != nil {
	//	return err
	//}
	return nil
}
