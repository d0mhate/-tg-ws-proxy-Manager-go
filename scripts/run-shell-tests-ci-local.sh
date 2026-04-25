#!/bin/sh

set -eu

IMAGE="${CI_SHELL_TESTS_IMAGE:-ubuntu:24.04}"
WORKDIR_IN_CONTAINER="/work"

docker run --rm \
  -v "$PWD":"$WORKDIR_IN_CONTAINER" \
  -w "$WORKDIR_IN_CONTAINER" \
  "$IMAGE" \
  bash -lc '
    set -eu
    export DEBIAN_FRONTEND=noninteractive

    apt-get update
    apt-get install -y --no-install-recommends \
      bash \
      ca-certificates \
      curl \
      git \
      make \
      tar

    BATS_VERSION="${BATS_VERSION:-1.13.0}"
    BATS_ARCHIVE="bats-core-v${BATS_VERSION}.tar.gz"
    BATS_URL="https://github.com/bats-core/bats-core/archive/refs/tags/v${BATS_VERSION}.tar.gz"
    BATS_DIR="/tmp/bats-core-${BATS_VERSION}"

    curl -fsSL "$BATS_URL" -o "/tmp/${BATS_ARCHIVE}"
    tar -xzf "/tmp/${BATS_ARCHIVE}" -C /tmp
    "/tmp/bats-core-${BATS_VERSION}/install.sh" /usr/local

    bats --version
    make test-shell-verbose
  '
