#!/bin/bash
set -e

export DEBIAN_FRONTEND=noninteractive
printf 'deb http://archive.debian.org/debian stretch main\ndeb http://archive.debian.org/debian-security stretch/updates main\n' > /etc/apt/sources.list
apt-get -o Acquire::Check-Valid-Until=false update -qq
apt-get install -y -qq --no-install-recommends --allow-unauthenticated \
  ca-certificates build-essential xorg-dev libasound2-dev libgl1-mesa-dev

export PATH=/usr/local/go/bin:$PATH
ldd --version | head -1
go version
go build -ldflags="-s -w" -o "/src/probe-out/${PROBE_LABEL}.bin" main.go keepawake_other.go
