#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
pushd ../cmd/kindest >/dev/null
    go build
popd >/dev/null
cd ..
cmd/kindest/kindest build
