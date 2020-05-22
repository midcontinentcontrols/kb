module github.com/midcontinentcontrols/kindest

go 1.14

require (
	github.com/Jeffail/tunny v0.0.0-20190930221602-f13eb662a36a
	github.com/Masterminds/semver v1.5.0 // indirect
	github.com/docker/docker v1.13.1
	github.com/google/go-cmp v0.4.1 // indirect
	github.com/google/uuid v1.1.1
	github.com/gorilla/mux v1.7.4 // indirect
	github.com/hashicorp/go-multierror v1.1.0
	github.com/jhoonb/archivex v0.0.0-20180718040744-0488e4ce1681
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/sirupsen/logrus v1.6.0 // indirect
	github.com/spf13/cobra v1.0.0
	github.com/stretchr/testify v1.5.1
	go.uber.org/zap v1.15.0
	golang.org/x/sys v0.0.0-20200519105757-fe76b779f299 // indirect
	golang.org/x/time v0.0.0-20200416051211-89c76fbcd5d1 // indirect
	google.golang.org/grpc v1.29.1 // indirect
	gopkg.in/yaml.v2 v2.3.0
	helm.sh/helm/v3 v3.2.1
	k8s.io/api v0.18.3
	k8s.io/apimachinery v0.18.3
	k8s.io/client-go v0.18.0
	k8s.io/helm v2.16.7+incompatible
	rsc.io/letsencrypt v0.0.3 // indirect
	sigs.k8s.io/kind v0.8.1
)

replace github.com/docker/docker => github.com/docker/engine v17.12.0-ce-rc1.0.20190717161051-705d9623b7c1+incompatible
