#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
cd ../cmd/kindest
go build
sudo mv kindest /usr/local/bin/kind-exp

