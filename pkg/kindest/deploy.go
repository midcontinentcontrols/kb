package kindest

type DeployOptions struct {
	KubeContext string `json:"kubeContext"`
}

func (m *Module) Deploy(options *DeployOptions) error {
	// TODO: move to deploy phase
	if err := applyManifests(
		options.KubeContext,
		m.Dir(),
		m.Spec.Env.Kubernetes.Resources,
	); err != nil {
		return err
	}
	//if err := t.installCharts(
	//	m.Dir(),
	//	options.KubeContext,
	//	log,
	//); err != nil {
	//	return err
	//}
	return nil
}
