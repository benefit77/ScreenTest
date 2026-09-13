#!/bin/bash
set -e

export DEBIAN_FRONTEND=noninteractive
# EOL 的 trusty/xenial 的 Release 文件 Valid-Until 已过期，这里不检查时效性。
apt-get -o Acquire::Check-Valid-Until=false update -qq
apt-get install -y -qq --no-install-recommends --allow-unauthenticated \
  ca-certificates build-essential xorg-dev libasound2-dev libgl1-mesa-dev

export PATH=/usr/local/go/bin:$PATH
ldd --version | head -1
go version
go build -ldflags="-s -w" -o "/src/probe-out/${PROBE_LABEL}.bin" main.go keepawake_other.go
