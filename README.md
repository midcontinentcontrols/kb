# Kindest DevOps for Monorepos

WIP. Expect nothing here to work.

This is a toolchain built on top of [kind](https://github.com/kubernetes-sigs/kind) that aims to reduce the complexity associated with using it as a tool for microservice development. It was born out of necessity to reduce increasing execution times and maintenance overhead of bash scripts that accomplished more or less the same thing.

At its core, the `kindest.yaml` file defines how images are built and tested in a transient Kubernetes cluster running either locally (with Docker daemon) or on another Kubernetes cluster (with some security caveats). The build process is fully parallelized and utilizes caching, so as to bypass redundant work for each submodule when no files have changed. Hooks are exposed for use with CI/CD.

### kindest.yaml
```yaml
# Relative paths to any dependent modules. Building or
# testing this module will do the same for all deps.
# These are built/tested concurrently, so it can be more
# generally exploited to parallelize build jobs.
dependencies:
  - my-submodule # ./my-submodule/kindest.yaml

build:
  # Name of the Docker image to build. The tag is chosen
  # by kindest, so only specify repo and image.
  name: midcontinentcontrols/example-image

  # This module is built by Dockerfile. By default, it
  # will search for a Dockerfile in the same directory
  # as this kindest.yaml.
  #dockerfile: ./Dockerfile

  # Docker build context, relative to kindest.yaml
  #context: .

  # https://docs.docker.com/engine/reference/commandline/build/
  #buildArgs:
  #  - name: ARG_NAME
  #    value: ARG_VALUE

test:
  # Tests have a `build` section mirroring the module's.
  # The image is automatically named. Typically, this
  # image will contain source code for all the monorepo's
  # dependencies and be multiple gb in size. 
  build:
    name: midcontinentcontrols/example-test
    dockerfile: test/Dockerfile
  
  # 
  env:
    #docker: {}
    kind:
      # These charts will be installed/upgraded when the
      # environment is setup.
      charts:
        - releaseName: kindest
          path: ./charts/kindest # ./charts/kindest/Chart.yaml
          values: {}
    # List of environment variables that will be passed to the test container.
    variables:
      - name: EXAMPLE_DEPENDENCY_URI
        value: http://example-dependency-microservice.default.svc.cluster.local:5000
```

## Features

### TODO: Automatic Dockerfile Generation
Additional work has gone into automatically generating efficient Dockerfiles for golang and Rust projects. These improvements automatically reduce the size of the build context.

### TODO: Modular Testing
A `kindest.yaml` file may define a minimalistic environment for end-to-end testing. The `test.env:` section dictates how the test pod couples with this environment. When a test is ran inside a given environment, it is passed these variables. This allows any module's environment to be used to test its dependencies, which is particularly useful when using transient clusters to test each commit.

### Transient & (TODO) Persistent Clusters
Test environments may exist either as an ephemeral cluster that is cleaned up when the tests finish or as a long-running cluster that persists between test runs. Persistent clusters are more performant and therefore recommended when running locally.

Currently, only transient clusters are supported.

## Running the Tests
To run the tests with full console output:
```
cd pkg/kindest
go test -v
```

## Docker Desktop Resource Limits
**The default resource limits for Docker Desktop appear insufficient to run the tests.** If this occurs, you will encounter [kind#1437](https://github.com/kubernetes-sigs/kind/issues/1437#issuecomment-602975739). Configure Docker with 4gb of both memory and swap just to be safe:

![](docs/images/docker-resources.png)

## Security
Running kind in a Kubernetes pod poses security risks worthy of operator attention. The Docker daemon of the node, running as root, is exposed to the test cluster. This is considered acceptable when running trusted code on dedicated hardware, which is the target use case of kindest. Open source developers in particular should consider the risks of using kindest with their community CI and take appropriate mitigating measures. 



## License
Copyright (c) Mid Continent Controls, Inc. 2020

Released under MIT and Apache dual licenses. Unencumbered commercial use is permitted. See LICENSE-Apache and LICENSE-MIT files for more information.
