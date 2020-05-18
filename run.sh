#!/bin/bash
set -euo pipefail
pushd cmd/kinder >/dev/null
    go build
popd >/dev/null
cmd/kinder/kinder build
