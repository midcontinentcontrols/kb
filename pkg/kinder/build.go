package kinder

func Build(spec *KinderSpec, rootPath string) error {
	if err := spec.Validate(rootPath); err != nil {
		return err
	}
	return nil
}
