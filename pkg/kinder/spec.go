package kinder

type DockerBuildArg struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type EnvSpec struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type DockerBuildSpec struct {
	Dockerfile string           `json:"dockerfile"`
	Context    string           `json:"context,omitempty"`
	BuildArgs  []DockerBuildArg `json:"buildArgs,omitempty"`
}

type BuildSpec struct {
	Docker *DockerBuildSpec `json:"docker,omitempty"`
}

type ChartSpec struct {
	ReleaseName string                 `json:"releaseName"`
	Path        string                 `json:"path"`
	Values      map[string]interface{} `json:"values,omitempty"`
}

type TestSpec struct {
	Charts []*ChartSpec `json:"charts"`
	Build  BuildSpec    `json:"build"`
	Env    []*EnvSpec   `json:"env,omitempty"`
}

type KinderSpec struct {
	Name         string    `json:"name"`
	Dependencies []string  `json:"dependencies,omitempty"`
	Build        BuildSpec `json:"build"`
	Test         TestSpec  `json:"test"`
}
