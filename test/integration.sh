#!/bin/sh
# Run end-to-end scenarios against real apt/dpkg in a disposable
# debian:trixie container. Usage: test/integration.sh path/to/debforge
set -eu
bin=$(realpath "${1:?usage: $0 path/to/debforge}")
here=$(dirname "$(realpath "$0")")
docker run --rm -i -e ITEST_SLOW="${ITEST_SLOW:-}" \
    -v "$bin:/usr/local/bin/debforge:ro" \
    -v "$here/scenarios.sh:/scenarios.sh:ro" \
    -v "$here/overlay:/etc/debforge/packages.d:ro" \
    debian:trixie sh /scenarios.sh
