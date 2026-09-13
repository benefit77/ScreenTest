#!/bin/bash
# 检查 Linux 产物能不能在很老的发行版上跑。
# 判定标准是二进制实际引用的最高 glibc 符号版本，跟编译机器无关，
# 但编译机器越老，编译器选到的符号版本通常越老。
#
# 用法:  ./verify-linux-bin.sh [产物路径] [glibc 上限，默认 GLIBC_2.14]
set -euo pipefail

bin="${1:-screen-tester-linux-x64}"
limit="${2:-GLIBC_2.14}"

if [ ! -f "$bin" ]; then
  echo "找不到文件: $bin" >&2
  exit 2
fi

class=$(readelf -h "$bin" | awk -F: '/Class:/{gsub(/ /,"",$2); print $2}')
machine=$(readelf -h "$bin" | awk -F: '/Machine:/{sub(/^ +/,"",$2); print $2}')
max=$(readelf --version-info "$bin" | grep -oE 'GLIBC_[0-9.]+' | sort -Vu | tail -1)
needed=$(readelf -d "$bin" | awk '/NEEDED/{gsub(/[\[\]]/,"",$NF); printf "%s ", $NF}')

echo "文件:       $bin"
echo "架构:       $class / $machine"
echo "glibc 需求: ${max:-无}"
echo "动态依赖:   ${needed:-无}"

rc=0
if [ "$class" != "ELF64" ]; then
  echo "[FAIL] 不是 64 位 ELF"
  rc=1
fi
if [ "$machine" != "Advanced Micro Devices X86-64" ]; then
  echo "[FAIL] 不是 x86-64: $machine"
  rc=1
fi
if [ -n "${max:-}" ]; then
  highest=$(printf '%s\n%s\n' "$limit" "$max" | sort -V | tail -1)
  if [ "$highest" != "$limit" ]; then
    echo "[FAIL] 产物要求 $max，高于上限 $limit"
    rc=1
  fi
fi

if [ "$rc" -eq 0 ]; then
  echo "[OK] 最高只需要 ${max:-无}，可运行于 Ubuntu 12.04+ / Debian 8+ / CentOS 7+ / RHEL 7+"
fi
exit "$rc"
