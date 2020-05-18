#!/bin/bash
set -euo pipefail
pushd cmd/kindest >/dev/null
    go build
popd >/dev/null
cmd/kindest/kindest build
