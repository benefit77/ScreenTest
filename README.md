# ScreenTest

屏幕测试小工具：纯色 / 渐变 / 随机闪烁画面，用来检查坏点、色偏，也支持拖动框测量和触摸定位。

## 两个版本

| 文件 | 适用系统 | 编译方式 |
| --- | --- | --- |
| ScreenTest.exe | Windows 7/10/11（32 位） | ebiten + cgo，见 workflow 的 `build-modern-windows` |
| ScreenTest_xp.exe | Windows XP / 2003（32 位） | 纯 Win32 GDI，用 `build-xp.ps1` |

XP 版没有用 ebiten —— ebiten 依赖新的图形接口，XP 上跑不起来，所以 `main_xp.go`
直接用 user32/gdi32 画图，通过 `//go:build xp` 标签和普通版区分开。

## 编译 XP 版

```powershell
.\build-xp.ps1 -DownloadGo108     # 自动下载 Go 1.10.8，编译并校验产物
```

XP 版本对编译器有三个硬性要求，脚本已经固定好：

1. 必须用 Go 1.10.8。Go 1.11 起最低要求 Windows 7，Go 1.21 起要求 Windows 10，
   用新 Go 编出来的 exe 子系统版本是 6.x，XP 会直接拒绝加载（典型现象就是双击没反应）。
2. 必须 GOARCH=386。XP 是 32 位系统，amd64 的 exe 会提示“不是有效的 Win32 应用程序”。
3. 必须 CGO_ENABLED=0。纯 Go 静态链接，不依赖 MSVCRT 等运行时库。

编译完可以单独校验产物：

```powershell
.\verify-xp-exe.ps1 -Path .\ScreenTest_xp.exe
```

校验内容包括：32 位 PE32 / i386、子系统版本不高于 5.1、没有静态导入 Vista/Win7 才有的函数。

## 操作方式（两个版本一致）

| 操作 | 效果 |
| --- | --- |
| 点击 / 空格 / 右方向键 | 下一个画面 |
| 左方向键 | 上一个画面 |
| F 键 | 随机闪烁模式（看响应和拖影） |
| 拖动 | 画框并显示像素尺寸 |
| 长按 2 秒 | 退出 |
| 右键 / ESC | 退出 |

运行期间会调用 `SetThreadExecutionState` 防止息屏，适合长时间烤机。

触摸这块两个版本不一样：Windows 7 及以上走 `WM_TOUCH` 原生触摸消息；
XP 没有这套 API，触摸屏一般是以鼠标形式上报，所以 XP 版走鼠标消息（拖动框、点击切画面都正常）。
