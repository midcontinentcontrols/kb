# Kinder DevOps for Monorepos

This is a toolchain built on top of [kind](https://github.com/kubernetes-sigs/kind) that aims to reduce the complexity associated with using it as a tool for microservice development. It was born out of necessity to reduce increasing execution times and maintenance overhead of bash scripts that accomplished more or less the same thing.

At its core, the `kinder.yaml` file defines how images are built and tested in a transient Kubernetes cluster running either locally (with Docker daemon) or on another Kubernetes cluster (with some security caveats). The build process is fully parallelized and utilizes caching, so as to bypass redundant work for each submodule when no files have changed. Hooks are exposed for use with CI/CD.

### kinder.yaml
```yaml
# Name of the Docker image to build. The tag is chosen
# by kinder, so only specify repo and image.
name: midcontinentcontrols/kinder

# Relative paths to any dependent modules. Building or
# testing this module will do the same for all deps.
# These are built/tested concurrently, so it can be more
# generally exploited to parallelize build jobs.
dependencies:
  - my-submodule # ./my-submodule/kinder.yaml

build:
  # This module is built by Dockerfile. By default, it
  # will search for a Dockerfile in the same directory
  # as this kinder.yaml.
  docker: {}
    #dockerfile: ./Dockerfile
    #context: .

    # Pass these to `docker build` as --build-arg 
    #buildArgs:
    #  - name: ARG_NAME
    #    value: ARG_VALUE

  # Automatically generates a Dockerfile from a Go module.
  # Import statements across all source files are recursively
  # followed, and any module within the monorepo but outside
  # of this module are added to the build context.
  # The generated Dockerfile will utilize the monorepo's
  # dependency cache - particularly useful for speeding up
  # local development.
  #go:
    ## Additionally directories to unconditionally include
    ## in the build context.
    #include: []

  # Automatically generates a Dockerfile for a Rust project.
  # Relative path dependencies within Cargo.toml are recursively
  # followed, and all modules within the monorepo but outside
  # of this module are added to the build context.
  #rust:
    ## Additionally directories to unconditionally include
    ## in the build context.
    #include: []

test:
  # These charts will be installed/upgraded when the
  # environment is setup.
  charts:
    - ./charts/kinder # ./charts/kinder/Chart.yaml

  # Tests have a `build` section mirroring the module's.
  # The image is automatically named. Typically, this
  # image will contain source code for all the monorepo's
  # dependencies and be multiple gb in size. 
  build:
    docker:
      dockerfile: test/Dockerfile

  spec:
    # List of environment variables that will be passed to
    # the test pod when the minimal environment is used. When
    # a parent environment is used, those variables will be
    # passed instead. Use this to couple the test pod to
    # the environment.
    env:
      - name: EXAMPLE_DEPENDENCY_URI
        value: http://example-dependency-microservice.default.svc.cluster.local:5000
```

## Features

### Automatic Dockerfile Generation
Additional work has gone into automatically generating efficient Dockerfiles for golang and Rust projects. These improvements automatically reduce the size of the build context.

### Modular Testing
A `kinder.yaml` file may define a minimalistic environment for end-to-end testing. The `test.env:` section dictates how the test pod couples with this environment. When a test is ran inside a given environment, it is passed these variables.

### Transient & Persistent Clusters
Test environments may exist either as an ephemeral cluster that is cleaned up when the tests finish or as a long-running cluster that persists between test runs. Persistent clusters are more performant and therefore recommended when running locally.

## Security
Running kind in a Kubernetes pod poses security risks worthy of operator attention. The Docker daemon of the node, running as root, is exposed to the test cluster. This is considered acceptable when running trusted code on dedicated hardware, which is the target use case of kinder. Open source developers in particular should consider the risks of using kinder with their community CI and take appropriate mitigating measures. 

## License
Copyright (c) Mid Continent Controls, Inc. 2020

Released under MIT and Apache dual licenses. Unencumbered commercial use is permitted. See LICENSE-Apache and LICENSE-MIT files for more information.
