#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
cd ../cmd/kb
go build
sudo mv kb /usr/local/bin/kb

