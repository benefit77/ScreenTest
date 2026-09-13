<#
  校验一个 exe 能不能在 32 位 Windows XP 上启动。

  XP 上打不开通常就三类原因，这个脚本逐条检查：
    1. 不是 32 位程序（amd64 的 exe 在 32 位 XP 上会提示“不是有效的 Win32 应用程序”）
    2. PE 子系统版本高于 5.1（Go 1.11 及以后编出来的 exe 是 6.x，XP 直接拒绝加载）
    3. 静态导入了 Vista/Win7 才有的函数（XP 会提示“无法定位程序输入点”）
       只看导入表，不看文件里的字符串 —— Go 运行时代码里本身就含这些名字。

  用法:  .\verify-xp-exe.ps1 -Path .\ScreenTest_xp.exe
#>
param(
    [Parameter(Mandatory = $true)]
    [string]$Path
)

$ErrorActionPreference = 'Stop'
$full = (Resolve-Path -LiteralPath $Path).Path
$bytes = [IO.File]::ReadAllBytes($full)

if ($bytes.Length -lt 0x100 -or [BitConverter]::ToUInt16($bytes, 0) -ne 0x5A4D) {
    throw "$Path 不是有效的 PE 文件"
}
$pe = [BitConverter]::ToInt32($bytes, 0x3C)
if ([BitConverter]::ToUInt32($bytes, $pe) -ne 0x00004550) {
    throw "$Path 不是有效的 PE 文件"
}

$machine = [BitConverter]::ToUInt16($bytes, $pe + 4)
$numSections = [BitConverter]::ToUInt16($bytes, $pe + 6)
$optSize = [BitConverter]::ToUInt16($bytes, $pe + 20)
$opt = $pe + 24
$magic = [BitConverter]::ToUInt16($bytes, $opt)
$subMajor = [BitConverter]::ToUInt16($bytes, $opt + 48)
$subMinor = [BitConverter]::ToUInt16($bytes, $opt + 50)
$subsystem = [BitConverter]::ToUInt16($bytes, $opt + 68)

$problems = @()
if ($magic -ne 0x10B) {
    $problems += ("不是 32 位 PE32（magic=0x{0:x}）——32 位 XP 只能运行 32 位程序" -f $magic)
}
if ($machine -ne 0x14C) {
    $problems += ("不是 i386 目标（machine=0x{0:x4}）——32 位 XP 只能运行 i386 的 exe" -f $machine)
}
if ($subMajor -gt 5 -or ($subMajor -eq 5 -and $subMinor -gt 1)) {
    $problems += "子系统版本 $subMajor.$subMinor 高于 Windows XP 的 5.1，XP 会拒绝加载（用 Go 1.11 及以上编译会出现这种情况）"
}
if ($subsystem -ne 2) {
    Write-Host '[提示] 子系统不是 GUI(2)，启动时会附带一个控制台窗口'
}

$sectionTable = $opt + $optSize
$sections = @()
for ($i = 0; $i -lt $numSections; $i++) {
    $o = $sectionTable + $i * 40
    $sections += [pscustomobject]@{
        VirtualSize = [BitConverter]::ToUInt32($bytes, $o + 8)
        VirtualAddr = [BitConverter]::ToUInt32($bytes, $o + 12)
        RawSize     = [BitConverter]::ToUInt32($bytes, $o + 16)
        RawPointer  = [BitConverter]::ToUInt32($bytes, $o + 20)
    }
}

function Get-FileOffsetFromRva {
    param([uint32]$Rva)
    foreach ($s in $sections) {
        $size = [Math]::Max($s.VirtualSize, $s.RawSize)
        if ($Rva -ge $s.VirtualAddr -and $Rva -lt ($s.VirtualAddr + $size)) {
            return [int]($s.RawPointer + ($Rva - $s.VirtualAddr))
        }
    }
    return -1
}

function Read-AsciiString {
    param([int]$Offset)
    $sb = New-Object System.Text.StringBuilder
    while ($Offset -lt $bytes.Length -and $bytes[$Offset] -ne 0) {
        [void]$sb.Append([char]$bytes[$Offset])
        $Offset++
    }
    return $sb.ToString()
}

$importedDlls = New-Object System.Collections.Generic.List[string]
if ($magic -eq 0x10B) {
    # 遍历导入表，只取真正静态链接进来的函数名
    $dataDir = $opt + 96
    $importRva = [BitConverter]::ToUInt32($bytes, $dataDir + 8)
    $importedFunctions = New-Object System.Collections.Generic.List[string]
    if ($importRva -ne 0) {
        $desc = Get-FileOffsetFromRva $importRva
        while ($desc -ge 0) {
            $originalFirstThunk = [BitConverter]::ToUInt32($bytes, $desc)
            $nameRva = [BitConverter]::ToUInt32($bytes, $desc + 12)
            $firstThunk = [BitConverter]::ToUInt32($bytes, $desc + 16)
            if ($originalFirstThunk -eq 0 -and $nameRva -eq 0 -and $firstThunk -eq 0) { break }
            if ($nameRva -ne 0) {
                $importedDlls.Add((Read-AsciiString (Get-FileOffsetFromRva $nameRva)))
                $thunkRva = if ($originalFirstThunk -ne 0) { $originalFirstThunk } else { $firstThunk }
                $thunkOffset = Get-FileOffsetFromRva $thunkRva
                while ($thunkOffset -ge 0) {
                    $value = [BitConverter]::ToUInt32($bytes, $thunkOffset)
                    if ($value -eq 0) { break }
                    if (($value -band 0x80000000) -eq 0) {
                        $nameOffset = Get-FileOffsetFromRva ($value + 2)
                        if ($nameOffset -ge 0) { $importedFunctions.Add((Read-AsciiString $nameOffset)) }
                    }
                    $thunkOffset += 4
                }
            }
            $desc += 20
        }
    }

    # 这些函数 Vista/Win7 才加入 kernel32，静态导入它们会导致 XP 报“无法定位程序输入点”
    $newerOnly = @(
        'WerSetFlags', 'WerGetFlags', 'GetQueuedCompletionStatusEx', 'CreateWaitableTimerExW',
        'InitializeCriticalSectionEx', 'GetSystemTimePreciseAsFileTime', 'RtlVirtualUnwind',
        'RaiseFailFastException', 'AddVectoredContinueHandler', 'SetThreadDescription',
        'GetCurrentPackageId', 'SetProcessMitigationPolicy'
    )
    foreach ($name in $newerOnly) {
        if ($importedFunctions -contains $name) {
            $problems += "静态导入了新系统才有的函数 $name（用较新的 Go 或开了 cgo 编译会出现这种情况）"
        }
    }
}

if ($problems.Count -gt 0) {
    $problems | ForEach-Object { Write-Host "X $_" }
    throw "$([IO.Path]::GetFileName($full)) 无法在 32 位 Windows XP 上运行"
}

$dllList = if ($importedDlls.Count -gt 0) { ($importedDlls | Sort-Object -Unique) -join ', ' } else { '无' }
Write-Host ("[OK] {0}: 32 位 PE32 / i386 / 子系统版本 {1}.{2} / subsystem={3} / {4:N0} 字节" -f `
        [IO.Path]::GetFileName($full), $subMajor, $subMinor, $subsystem, $bytes.Length)
Write-Host ("     静态依赖: {0}" -f $dllList)
