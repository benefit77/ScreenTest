#!/bin/bash
# 编译 Linux 版（64 位）。在哪个发行版里跑，决定了产物要求多新的 glibc：
# 编译环境的 glibc 越老，能跑的发行版越多，所以 CI 故意用 ubuntu:16.04 容器来编。
#
# 用法（容器里，挂载源码到 /src）:
#   docker run --rm -v "$PWD:/src" -v /tmp/go:/usr/local/go:ro -e GOPATH=/gopath \
#     -e CGO_ENABLED=1 -e GOOS=linux -e GOARCH=amd64 -w /src \
#     ubuntu:16.04 bash /src/build-linux.sh
set -e

cd "$(dirname "$0")"
export PATH=/usr/local/go/bin:$PATH

if command -v apt-get >/dev/null 2>&1; then
  export DEBIAN_FRONTEND=noninteractive
  # EOL 发行版的 Release 文件已过期，这里不检查时效性
  apt-get -o Acquire::Check-Valid-Until=false update -qq
  apt-get install -y -qq --no-install-recommends --allow-unauthenticated \
    ca-certificates build-essential xorg-dev libasound2-dev libgl1-mesa-dev
fi

echo "编译环境: $(. /etc/os-release && echo "$PRETTY_NAME") / $(ldd --version | head -1)"
echo "Go:       $(go version)"

go build -ldflags="-s -w" -o screen-tester-linux-x64 main.go keepawake_other.go

echo "产物:     $(ls -l screen-tester-linux-x64 | awk '{print $5" 字节"}')"

# 容器里是以 root 编译的，把属主改回宿主用户，方便后续步骤（UPX、上传）继续处理
if [ -f go.mod ]; then
  chown --reference=go.mod screen-tester-linux-x64 2>/dev/null || true
fi
