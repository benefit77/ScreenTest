<#
  编译 Windows 通用版：32 位 exe，XP / 2003 / 7 / 10 / 11 都能运行。

  为什么必须这么编：
    * Go 1.11 起最低要求 Windows 7，Go 1.21 起要求 Windows 10，
      所以想兼容 XP 就只能用最后一个支持 XP 的 Go 1.10.8；
      编出来的 exe 子系统版本是 4.0，XP 以上的系统也都接受。
    * XP 是 32 位系统，必须 GOARCH=386（默认的 amd64 在 XP 上打不开）。
    * XP 版走的是 GDI，不需要 ebiten，所以 CGO_ENABLED=0（不依赖 MSVCRT）。

  用法:
    .\build-xp.ps1                              # 用 PATH 里的 go（必须是 1.10.x）
    .\build-xp.ps1 -GoRoot E:\toolchains\go      # 指定 Go 1.10.8 目录
    .\build-xp.ps1 -DownloadGo108                # 自动下载 Go 1.10.8 再编译
    .\build-xp.ps1 -Output screen-tester-windows.exe
#>
param(
    [string]$GoRoot = $env:GO108_ROOT,
    [string]$Output = 'screen-tester-windows.exe',
    [switch]$DownloadGo108
)

$ErrorActionPreference = 'Stop'
$repo = $PSScriptRoot
$go108Url = 'https://dl.google.com/go/go1.10.8.windows-amd64.zip'

function Test-Go108([string]$exe) {
    if (-not $exe -or -not (Test-Path -LiteralPath $exe)) { return $false }
    return ((& $exe version) -match 'go1\.10\.')
}

if ($GoRoot) {
    $go = Join-Path $GoRoot 'bin\go.exe'
    if (-not (Test-Path -LiteralPath $go)) { throw "在 $GoRoot 下找不到 bin\go.exe" }
}
else {
    $cmd = Get-Command go.exe -ErrorAction SilentlyContinue
    $go = if ($cmd) { $cmd.Source } else { $null }
}

if ($DownloadGo108 -and -not (Test-Go108 $go)) {
    $target = if ($env:GO108_ROOT) { $env:GO108_ROOT } else { Join-Path $repo '.toolchains\go1.10.8' }
    Write-Host "下载 Go 1.10.8 到 $target ..."
    $zip = Join-Path ([IO.Path]::GetTempPath()) 'go1.10.8.windows-amd64.zip'
    if (-not (Test-Path -LiteralPath $zip)) {
        Invoke-WebRequest -Uri $go108Url -OutFile $zip
    }
    New-Item -ItemType Directory -Force -Path $target | Out-Null
    Expand-Archive -Path $zip -DestinationPath $target -Force
    $go = Join-Path $target 'go\bin\go.exe'
}

if (-not (Test-Go108 $go)) {
    $found = if ($go) { (& $go version) } else { '未找到 go' }
    throw @"
当前编译器是 "$found"，不能用来编 XP 版本：
Go 1.11 起编出来的 exe 要求 Windows 7+，Go 1.21 起要求 Windows 10，XP 上会直接打不开。
请用 Go 1.10.8 编译，二选一：
  .\build-xp.ps1 -DownloadGo108
  手工下载 $go108Url 解压后执行  .\build-xp.ps1 -GoRoot <解压目录>\go
"@
}

$env:GOOS = 'windows'
$env:GOARCH = '386'
$env:CGO_ENABLED = '0'
$env:GO111MODULE = 'off'

$out = if ([IO.Path]::IsPathRooted($Output)) { $Output } else { Join-Path $repo $Output }
Write-Host "编译器: $(& $go version)"
Write-Host "输出:   $out"
& $go build -tags xp -ldflags '-s -w -H=windowsgui' -o $out (Join-Path $repo 'main_xp.go')
if ($LASTEXITCODE -ne 0) { throw "编译失败（exit=$LASTEXITCODE）" }

& (Join-Path $repo 'verify-xp-exe.ps1') -Path $out
