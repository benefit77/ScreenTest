#!/bin/bash
set -e

sed -i 's|^mirrorlist=|#mirrorlist=|; s|^#baseurl=http://mirror.centos.org|baseurl=http://vault.centos.org|' /etc/yum.repos.d/CentOS-*.repo
yum install -y -q gcc make \
  libX11-devel libXcursor-devel libXrandr-devel libXinerama-devel libXi-devel libXxf86vm-devel \
  alsa-lib-devel

export PATH=/usr/local/go/bin:$PATH
ldd --version | head -1
go version
go build -ldflags="-s -w" -o "/src/probe-out/${PROBE_LABEL}.bin" main.go keepawake_other.go
