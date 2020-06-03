package kindest

import "fmt"

func (b *BuildSpec) buildKanikoLocal(
	manifestPath string,
	options *BuildOptions,
) error {
	return nil
}

func (b *BuildSpec) buildKanikoRemote(
	context string,
	manifestPath string,
	options *BuildOptions,
) error {
	return fmt.Errorf("unimplemented")
}

func (b *BuildSpec) buildKaniko(
	manifestPath string,
	options *BuildOptions,
) error {
	if options.Context != "" {
		return fmt.Errorf("unimplemented")
	}
	return b.buildKanikoLocal(manifestPath, options)
}
