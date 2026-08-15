//go:build xp
// +build xp

package main

import (
	"fmt"
	"math"
	"runtime"
	"syscall"
	"unsafe"
)

// 兼容 XP：本文件仅在 -tags xp 构建时编译

const (
	clickThresholdXP = 10 // 拖动超过此像素则算拖动，不触发点击

	holdExitMsXP     = 2000 // 长按此毫秒数退出
	holdCancelDistXP = 15   // 长按期间手指移动超过此像素则取消

	modeCountXP = 9
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	colors   = []uint32{0x0000FF, 0x00FF00, 0xFFFFFF, 0x000000, 0xFF0000, 0x00FFFF, 0xFF00FF}
	idx      = 0
	flashing = false

	// 拖动框
	dragStartX, dragStartY int32
	dragEndX, dragEndY     int32
	isDragging             bool

	// 长按退出
	holdActive    bool
	holdStartTick uint32
	holdX, holdY  int32

	// 缓存屏幕尺寸（启动时计算一次）
	screenW, screenH       int32
	horzSizeMm, vertSizeMm int32

	// 主循环拖动绘图（回调只设变量，不调任何 DLL）
	mouseCurX, mouseCurY int32
	dragEnded            bool
	hwndGlobal           uintptr
)

// Windows Touch 相关常量
const (
	WM_TOUCH         = 0x0240
	TOUCHEVENTF_DOWN = 0x0002
	TOUCHEVENTF_UP   = 0x0004
	TOUCHEVENTF_MOVE = 0x0001
	TWF_WANTPALM     = 0x00000002
)

// TOUCHINPUT 结构体（对应 Windows TOUCHINPUT）
type TOUCHINPUT struct {
	x           int32
	y           int32
	hSource     uintptr
	dwID        uint32
	dwFlags     uint32
	dwMask      uint32
	dwTime      uint32
	dwExtraInfo uintptr
	cxContact   uint32
	cyContact   uint32
}

// 预分配触摸输入缓冲区（最多支持 32 点同时触摸），避免回调中 make 分配内存
var touchBuf [32]TOUCHINPUT

// 只让拖动框经过的区域失效重绘（旧框 + 新框并集），避免滑动时全屏重绘导致卡顿
func invalidateBoxRange(hwnd uintptr, oldX, oldY, newX, newY int32) {
	sx, sy := dragStartX, dragStartY
	ox1, oy1, ox2, oy2 := sx, sy, oldX, oldY
	if ox1 > ox2 {
		ox1, ox2 = ox2, ox1
	}
	if oy1 > oy2 {
		oy1, oy2 = oy2, oy1
	}
	nx1, ny1, nx2, ny2 := sx, sy, newX, newY
	if nx1 > nx2 {
		nx1, nx2 = nx2, nx1
	}
	if ny1 > ny2 {
		ny1, ny2 = ny2, ny1
	}
	left := ox1
	if nx1 < left {
		left = nx1
	}
	top := oy1
	if ny1 < top {
		top = ny1
	}
	right := ox2
	if nx2 > right {
		right = nx2
	}
	bottom := oy2
	if ny2 > bottom {
		bottom = ny2
	}
	// 留边距：边框宽度、角标、尺寸文字
	r := [4]int32{left - 8, top - 24, right + 10, bottom + 8}
	if r[0] < 0 {
		r[0] = 0
	}
	if r[1] < 0 {
		r[1] = 0
	}
	if r[2] > screenW {
		r[2] = screenW
	}
	if r[3] > screenH {
		r[3] = screenH
	}
	if r[2] <= r[0] || r[3] <= r[1] {
		return
	}
	user32.NewProc("InvalidateRect").Call(hwnd, uintptr(unsafe.Pointer(&r)), 0)
}

func wndProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case 0x0001: // WM_CREATE
		user32.NewProc("ShowCursor").Call(0)
		return 0
	case 0x0002: // WM_DESTROY
		user32.NewProc("PostQuitMessage").Call(0)
		return 0
	case 0x0100: // WM_KEYDOWN
		switch wp {
		case 0x46: // F 键 - 开启/关闭闪烁
			flashing = !flashing
			if flashing {
				user32.NewProc("SetTimer").Call(hwnd, 1, 30, 0)
			} else {
				user32.NewProc("KillTimer").Call(hwnd, 1)
			}
		case 0x27, 0x20, 0x0D: // Right, Space, Enter
			idx++
			if idx >= modeCountXP {
				syscall.Exit(0)
			}
		case 0x25: // Left
			idx--
			if idx < 0 {
				idx = 0
			}
		case 0x1B: // ESC
			syscall.Exit(0)
		}
		user32.NewProc("InvalidateRect").Call(hwnd, 0, 1)
	case 0x0113: // WM_TIMER
		if flashing {
			user32.NewProc("InvalidateRect").Call(hwnd, 0, 1)
		}
		if holdActive {
			user32.NewProc("InvalidateRect").Call(hwnd, 0, 1)
		}
		if wp == 3 {
			// 防息屏：定期重新保持系统与屏幕唤醒
			kernel32.NewProc("SetThreadExecutionState").Call(uintptr(0x80000000 | 0x00000001 | 0x00000002))
		}
	case 0x0200: // WM_MOUSEMOVE - 只设变量，不调任何 DLL
		mouseCurX = int32(int16(lp & 0xFFFF))
		mouseCurY = int32(int16((lp >> 16) & 0xFFFF))
		if isDragging {
			// 拖动中：更新拖动框终点，让框跟随指针
			oldX, oldY := dragEndX, dragEndY
			dragEndX, dragEndY = mouseCurX, mouseCurY
			invalidateBoxRange(hwnd, oldX, oldY, mouseCurX, mouseCurY)
			if holdActive {
				dx := mouseCurX - holdX
				dy := mouseCurY - holdY
				if dx*dx+dy*dy > holdCancelDistXP*holdCancelDistXP {
					// 手指移动了：取消长按（当作拖动）
					holdActive = false
					user32.NewProc("KillTimer").Call(hwnd, 2)
				}
			}
		}
	case 0x0201: // WM_LBUTTONDOWN - 只设变量
		rawX := int32(int16(lp & 0xFFFF))
		rawY := int32(int16((lp >> 16) & 0xFFFF))
		dragStartX, dragStartY = rawX, rawY
		dragEndX, dragEndY = dragStartX, dragStartY
		isDragging = true
		holdActive = true
		tick, _, _ := kernel32.NewProc("GetTickCount").Call()
		holdStartTick = uint32(tick)
		holdX, holdY = dragStartX, dragStartY
		user32.NewProc("SetTimer").Call(hwnd, 2, 50, 0)
		invalidateBoxRange(hwnd, dragStartX, dragStartY, dragStartX, dragStartY)
		return 0
	case 0x0202: // WM_LBUTTONUP - 只设变量
		if isDragging {
			isDragging = false
			holdActive = false
			user32.NewProc("KillTimer").Call(hwnd, 2)
			dragEnded = true
			// 记录鼠标最终位置
			dragEndX = int32(int16(lp & 0xFFFF))
			dragEndY = int32(int16((lp >> 16) & 0xFFFF))
		}
		return 0
	case 0x0204:
		syscall.Exit(0) // Right Click
		return 0
	case WM_TOUCH: // 触摸事件
		numInputs := int(wp)
		if numInputs > 0 && numInputs <= 32 {
			// 使用预分配缓冲区，避免 make()
			getTouch, _, _ := user32.NewProc("GetTouchInputInfo").Call(lp, uintptr(numInputs), uintptr(unsafe.Pointer(&touchBuf[0])), uintptr(unsafe.Sizeof(TOUCHINPUT{})))
			if getTouch != 0 {
				needRepaint := false
				for i := 0; i < numInputs; i++ {
					ti := &touchBuf[i]
					// himetric -> 像素（使用缓存的屏幕尺寸）
					px, py := ti.x, ti.y
					if horzSizeMm > 0 {
						px = ti.x * screenW / (horzSizeMm * 100)
					}
					if vertSizeMm > 0 {
						py = ti.y * screenH / (vertSizeMm * 100)
					}
					if ti.dwFlags&TOUCHEVENTF_UP != 0 {
						isDragging = false
						holdActive = false
						user32.NewProc("KillTimer").Call(hwnd, 2)
						needRepaint = true

						dx := dragEndX - dragStartX
						dy := dragEndY - dragStartY
						distSq := dx*dx + dy*dy

						if distSq < clickThresholdXP*clickThresholdXP && !flashing {
							idx++
							if idx >= modeCountXP {
								syscall.Exit(0)
							}
						}
					} else if ti.dwFlags&TOUCHEVENTF_DOWN != 0 {
						dragStartX, dragStartY = px, py
						dragEndX, dragEndY = px, py
						isDragging = true
						holdActive = true
						tick, _, _ := kernel32.NewProc("GetTickCount").Call()
						holdStartTick = uint32(tick)
						holdX, holdY = px, py
						user32.NewProc("SetTimer").Call(hwnd, 2, 50, 0)
						needRepaint = true
					} else if ti.dwFlags&TOUCHEVENTF_MOVE != 0 && isDragging {
						if dragEndX != px || dragEndY != py {
							oldX, oldY := dragEndX, dragEndY
							dragEndX, dragEndY = px, py
							invalidateBoxRange(hwnd, oldX, oldY, px, py)
							if holdActive {
								dx := px - holdX
								dy := py - holdY
								if dx*dx+dy*dy > holdCancelDistXP*holdCancelDistXP {
									holdActive = false
									user32.NewProc("KillTimer").Call(hwnd, 2)
								}
							}
						}
					}
				}
				if needRepaint {
					user32.NewProc("InvalidateRect").Call(hwnd, 0, 1)
				}
			}
			user32.NewProc("CloseTouchInputHandle").Call(lp)
		}
		return 0
	case 0x000F: // WM_PAINT
		// PAINTSTRUCT: hdc(BOOL), fErase(RECT), rcPaint(BOOL), fRestore(BOOL), fIncUpdate(BYTE[32]), rgbReserved
		var ps struct {
			hdc      uintptr
			fErase   uint32
			rc       [4]int32
			fRestore uint32
			fIncUpd  uint32
			res      [32]byte
		}
		hdc, _, _ := user32.NewProc("BeginPaint").Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		w, h := uintptr(screenW), uintptr(screenH)

		if flashing {
			// 闪烁模式：用 GetTickCount 生成伪随机颜色
			tick, _, _ := kernel32.NewProc("GetTickCount").Call()
			c := uint32(tick) & 0xFFFFFF
			brush, _, _ := gdi32.NewProc("CreateSolidBrush").Call(uintptr(c))
			rect := [4]int32{0, 0, int32(w), int32(h)}
			user32.NewProc("FillRect").Call(hdc, uintptr(unsafe.Pointer(&rect)), brush)
			gdi32.NewProc("DeleteObject").Call(brush)
		} else {
			// 正常模式逻辑（与普通版一致）
			if idx == 0 { // 彩条（白/黄/青/绿/品红/红/蓝）
				bars := []uint32{0xFFFFFF, 0x00FFFF, 0xFFFF00, 0x00FF00, 0xFF00FF, 0x0000FF, 0xFF0000}
				for i := 0; i < len(bars); i++ {
					r := [4]int32{int32(i * int(w) / len(bars)), 0, int32((i + 1) * int(w) / len(bars)), int32(h)}
					brush, _, _ := gdi32.NewProc("CreateSolidBrush").Call(uintptr(bars[i]))
					user32.NewProc("FillRect").Call(hdc, uintptr(unsafe.Pointer(&r)), brush)
					gdi32.NewProc("DeleteObject").Call(brush)
				}
			} else if idx >= 1 && idx <= len(colors) {
				rect := [4]int32{0, 0, int32(w), int32(h)}
				brush, _, _ := gdi32.NewProc("CreateSolidBrush").Call(uintptr(colors[idx-1]))
				user32.NewProc("FillRect").Call(hdc, uintptr(unsafe.Pointer(&rect)), brush)
				gdi32.NewProc("DeleteObject").Call(brush)
			} else { // 渐变（与普通版一致）
				for i := 0; i < 255; i++ {
					r := [4]int32{int32(i * int(w) / 255), 0, int32((i + 1) * int(w) / 255), int32(h)}
					c := uint32(i | (i << 8) | (i << 16))
					brush, _, _ := gdi32.NewProc("CreateSolidBrush").Call(uintptr(c))
					user32.NewProc("FillRect").Call(hdc, uintptr(unsafe.Pointer(&r)), brush)
					gdi32.NewProc("DeleteObject").Call(brush)
				}
			}
		}

		// ---- 拖动框（蓝边框 + 白色角标 + 尺寸文字） ----
		if isDragging {
			x1, y1 := dragStartX, dragStartY
			x2, y2 := dragEndX, dragEndY
			if x1 > x2 {
				x1, x2 = x2, x1
			}
			if y1 > y2 {
				y1, y2 = y2, y1
			}
			bw := x2 - x1
			bh := y2 - y1
			if bw > 0 && bh > 0 {
				// 蓝色实线边框（2px）
				bluePen, _, _ := gdi32.NewProc("CreatePen").Call(0, 2, 0x00FF9664)
				oldPen, _, _ := gdi32.NewProc("SelectObject").Call(hdc, bluePen)
				hNullBrush, _, _ := gdi32.NewProc("GetStockObject").Call(5) // NULL_BRUSH
				oldBrush, _, _ := gdi32.NewProc("SelectObject").Call(hdc, hNullBrush)
				gdi32.NewProc("Rectangle").Call(hdc, uintptr(x1), uintptr(y1), uintptr(x2+1), uintptr(y2+1))
				gdi32.NewProc("SelectObject").Call(hdc, oldBrush)
				gdi32.NewProc("SelectObject").Call(hdc, oldPen)
				gdi32.NewProc("DeleteObject").Call(bluePen)
			}

			// 白色角标
			whitePen, _, _ := gdi32.NewProc("CreatePen").Call(0, 2, 0x00FFFFFF)
			oldPen2, _, _ := gdi32.NewProc("SelectObject").Call(hdc, whitePen)
			cornerLen := int32(12)
			// 左上角
			gdi32.NewProc("MoveToEx").Call(hdc, uintptr(x1), uintptr(y1), 0)
			gdi32.NewProc("LineTo").Call(hdc, uintptr(x1+cornerLen), uintptr(y1))
			gdi32.NewProc("MoveToEx").Call(hdc, uintptr(x1), uintptr(y1), 0)
			gdi32.NewProc("LineTo").Call(hdc, uintptr(x1), uintptr(y1+cornerLen))
			// 右上角
			gdi32.NewProc("MoveToEx").Call(hdc, uintptr(x2), uintptr(y1), 0)
			gdi32.NewProc("LineTo").Call(hdc, uintptr(x2-cornerLen), uintptr(y1))
			gdi32.NewProc("MoveToEx").Call(hdc, uintptr(x2), uintptr(y1), 0)
			gdi32.NewProc("LineTo").Call(hdc, uintptr(x2), uintptr(y1+cornerLen))
			// 左下角
			gdi32.NewProc("MoveToEx").Call(hdc, uintptr(x1), uintptr(y2), 0)
			gdi32.NewProc("LineTo").Call(hdc, uintptr(x1+cornerLen), uintptr(y2))
			gdi32.NewProc("MoveToEx").Call(hdc, uintptr(x1), uintptr(y2), 0)
			gdi32.NewProc("LineTo").Call(hdc, uintptr(x1), uintptr(y2-cornerLen))
			// 右下角
			gdi32.NewProc("MoveToEx").Call(hdc, uintptr(x2), uintptr(y2), 0)
			gdi32.NewProc("LineTo").Call(hdc, uintptr(x2-cornerLen), uintptr(y2))
			gdi32.NewProc("MoveToEx").Call(hdc, uintptr(x2), uintptr(y2), 0)
			gdi32.NewProc("LineTo").Call(hdc, uintptr(x2), uintptr(y2-cornerLen))
			gdi32.NewProc("SelectObject").Call(hdc, oldPen2)
			gdi32.NewProc("DeleteObject").Call(whitePen)

			// 尺寸文字
			font, _, _ := gdi32.NewProc("GetStockObject").Call(17) // DEFAULT_GUI_FONT
			gdi32.NewProc("SelectObject").Call(hdc, font)
			gdi32.NewProc("SetBkMode").Call(hdc, 1) // TRANSPARENT
			gdi32.NewProc("SetTextColor").Call(hdc, 0x00FFFFFF)
			label, _ := syscall.UTF16FromString(fmt.Sprintf("%d x %d", bw, bh))
			if y1-16 >= 0 {
				gdi32.NewProc("TextOutW").Call(hdc, uintptr(x1+4), uintptr(y1-16), uintptr(unsafe.Pointer(&label[0])), uintptr(len(label)))
			} else {
				gdi32.NewProc("TextOutW").Call(hdc, uintptr(x1+4), uintptr(y1+4), uintptr(unsafe.Pointer(&label[0])), uintptr(len(label)))
			}
		}

		// ---- 长按退出进度圈 ----
		if holdActive {
			tick, _, _ := kernel32.NewProc("GetTickCount").Call()
			elapsed := uint32(tick) - holdStartTick
			prog := float64(elapsed) / holdExitMsXP
			if prog > 1 {
				prog = 1
			}
			// 背景圈
			ringPen, _, _ := gdi32.NewProc("CreatePen").Call(0, 2, 0x00707070)
			oldRingPen, _, _ := gdi32.NewProc("SelectObject").Call(hdc, ringPen)
			ringNullBrush, _, _ := gdi32.NewProc("GetStockObject").Call(5) // NULL_BRUSH
			oldRingBrush, _, _ := gdi32.NewProc("SelectObject").Call(hdc, ringNullBrush)
			gdi32.NewProc("Ellipse").Call(hdc, uintptr(holdX-36), uintptr(holdY-36), uintptr(holdX+36), uintptr(holdY+36))
			gdi32.NewProc("SelectObject").Call(hdc, oldRingBrush)
			gdi32.NewProc("SelectObject").Call(hdc, oldRingPen)
			gdi32.NewProc("DeleteObject").Call(ringPen)

			// 进度分段（24 段）
			cyanPen, _, _ := gdi32.NewProc("CreatePen").Call(0, 3, 0x00FFDC00) // 青色 RGB(0,220,255)
			oldCyanPen, _, _ := gdi32.NewProc("SelectObject").Call(hdc, cyanPen)
			const segs = 24
			lit := int(prog * segs)
			for i := 0; i < lit; i++ {
				a0 := -math.Pi/2 + float64(i)*2*math.Pi/segs
				a1 := -math.Pi/2 + float64(i+1)*2*math.Pi/segs
				x0 := holdX + int32(36*math.Cos(a0))
				y0 := holdY + int32(36*math.Sin(a0))
				x1 := holdX + int32(36*math.Cos(a1))
				y1 := holdY + int32(36*math.Sin(a1))
				gdi32.NewProc("MoveToEx").Call(hdc, uintptr(x0), uintptr(y0), 0)
				gdi32.NewProc("LineTo").Call(hdc, uintptr(x1), uintptr(y1))
			}
			gdi32.NewProc("SelectObject").Call(hdc, oldCyanPen)
			gdi32.NewProc("DeleteObject").Call(cyanPen)

			// 提示文字
			font, _, _ := gdi32.NewProc("GetStockObject").Call(17) // DEFAULT_GUI_FONT
			gdi32.NewProc("SelectObject").Call(hdc, font)
			gdi32.NewProc("SetBkMode").Call(hdc, 1) // TRANSPARENT
			gdi32.NewProc("SetTextColor").Call(hdc, 0x00FFFFFF)
			hint, _ := syscall.UTF16FromString("release to cancel")
			gdi32.NewProc("TextOutW").Call(hdc, uintptr(holdX-44), uintptr(holdY+44), uintptr(unsafe.Pointer(&hint[0])), uintptr(len(hint)))
		}

		user32.NewProc("EndPaint").Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	}
	ret, _, _ := user32.NewProc("DefWindowProcW").Call(hwnd, uintptr(msg), wp, lp)
	return ret
}

func main() {
	// 锁定主线程：避免 Go 调度器在 GetMessage/DispatchMessage 之间迁移线程，
	// 否则大量消息（如快速触摸滑动）时会触发 Go 回调机制死锁，程序卡死
	runtime.LockOSThread()

	// 检测 Windows 版本，XP (5.x) 不支持 DPI 感知
	ver, _, _ := kernel32.NewProc("GetVersion").Call()
	verMajor := byte(ver) & 0xFF
	// Vista = 6.0, XP = 5.1, 从 Vista 及以上启用 DPI 感知
	if verMajor >= 6 {
		shcore := syscall.NewLazyDLL("shcore.dll")
		if shcore.Load() == nil {
			// Process_Per_Monitor_DPI_Aware = 2
			shcore.NewProc("SetProcessDpiAwareness").Call(2)
		} else {
			user32.NewProc("SetProcessDPIAware").Call()
		}
	}

	inst, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	cls, _ := syscall.UTF16PtrFromString("XP_ST_PRO")
	cur, _, _ := user32.NewProc("LoadCursorW").Call(0, 32512)

	var wc struct {
		cb              uint32
		st              uint32
		pr              uintptr
		cl, wd          int32
		ins, ic, cu, bg uintptr
		mn, cn          *uint16
		is              uintptr
	}
	wc.cb = uint32(unsafe.Sizeof(wc))
	wc.pr = syscall.NewCallback(wndProc)
	wc.ins = inst
	wc.cu = cur
	wc.cn = cls
	user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc)))

	w, _, _ := user32.NewProc("GetSystemMetrics").Call(0)
	h, _, _ := user32.NewProc("GetSystemMetrics").Call(1)

	hwnd, _, _ := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(cls)), 0, 0x80000000|0x10000000, 0, 0, w, h, 0, 0, inst, 0)

	// 注册触摸窗口（Windows 7 及以上支持）
	user32.NewProc("RegisterTouchWindow").Call(hwnd, TWF_WANTPALM)

	// 缓存屏幕物理尺寸（himetric->像素换算用）
	screenW = int32(w)
	screenH = int32(h)
	hdcScreen, _, _ := user32.NewProc("GetDC").Call(0)
	hs, _, _ := gdi32.NewProc("GetDeviceCaps").Call(hdcScreen, 4) // HORZSIZE
	vs, _, _ := gdi32.NewProc("GetDeviceCaps").Call(hdcScreen, 6) // VERTSIZE

	user32.NewProc("ReleaseDC").Call(0, hdcScreen)
	horzSizeMm = int32(hs)
	vertSizeMm = int32(vs)

	hwndGlobal = hwnd

	// 防息屏：运行期间防止系统休眠和屏幕关闭，并每 2 秒重新保持
	kernel32.NewProc("SetThreadExecutionState").Call(uintptr(0x80000000 | 0x00000001 | 0x00000002))
	user32.NewProc("SetTimer").Call(hwnd, 3, 2000, 0)

	var m struct {
		hwnd    uintptr
		message uint32
		wp, lp  uintptr
		t       uint32
		pt      struct{ x, y int32 }
	}
	getMsg := user32.NewProc("GetMessageW")
	dispatchMsg := user32.NewProc("DispatchMessageW")

	for {
		// 标准阻塞式消息获取
		r, _, _ := getMsg.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if r == 0 {
			break
		}
		dispatchMsg.Call(uintptr(unsafe.Pointer(&m)))

		// 长按 2 秒退出
		if holdActive {
			tick, _, _ := kernel32.NewProc("GetTickCount").Call()
			if uint32(tick)-holdStartTick >= holdExitMsXP {
				syscall.Exit(0)
			}
		}

		if dragEnded {
			dragEnded = false
			user32.NewProc("InvalidateRect").Call(hwnd, 0, 1)
			dx := dragEndX - dragStartX
			dy := dragEndY - dragStartY
			distSq := dx*dx + dy*dy
			if distSq < clickThresholdXP*clickThresholdXP && !flashing {
				idx++
				if idx >= modeCountXP {
					syscall.Exit(0)
				}
			}
			user32.NewProc("InvalidateRect").Call(hwnd, 0, 1)
		}
	}
}
