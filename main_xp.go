//go:build xp
// +build xp

package main

import (
	"syscall"
	"time"
	"unsafe"
)

// 兼容 XP：此文件仅在使用 -tags xp 构建时编译

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	colors   = []uint32{0x0000FF, 0x00FF00, 0xFFFFFF, 0x000000, 0xFF0000, 0x00FFFF, 0xFF00FF}
	idx      = 0
	flashing = false
)

func wndProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case 0x0001: // WM_CREATE
		user32.NewProc("ShowCursor").Call(0)
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
			if idx >= len(colors)+2 {
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
	case 0x0201: // Left Click
		idx++
		if idx >= len(colors)+2 {
			syscall.Exit(0)
		}
		user32.NewProc("InvalidateRect").Call(hwnd, 0, 1)
	case 0x0204:
		syscall.Exit(0) // Right Click
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
		w, _, _ := user32.NewProc("GetSystemMetrics").Call(0)
		h, _, _ := user32.NewProc("GetSystemMetrics").Call(1)

		if flashing {
			// 闪烁模式：使用随机颜色
			c := uint32(time.Now().UnixNano()) & 0xFFFFFF
			brush, _, _ := gdi32.NewProc("CreateSolidBrush").Call(uintptr(c))
			rect := [4]int32{0, 0, int32(w), int32(h)}
			user32.NewProc("FillRect").Call(hdc, uintptr(unsafe.Pointer(&rect)), brush)
			gdi32.NewProc("DeleteObject").Call(brush)
		} else {
			// 正常模式逻辑（同前）
			if idx < len(colors) {
				rect := [4]int32{0, 0, int32(w), int32(h)}
				brush, _, _ := gdi32.NewProc("CreateSolidBrush").Call(uintptr(colors[idx]))
				user32.NewProc("FillRect").Call(hdc, uintptr(unsafe.Pointer(&rect)), brush)
				gdi32.NewProc("DeleteObject").Call(brush)
			} else if idx == len(colors) { // 网格
				for i := 0; i <= 20; i++ {
					gdi32.NewProc("MoveToEx").Call(hdc, uintptr(i*int(w)/20), 0, 0)
					gdi32.NewProc("LineTo").Call(hdc, uintptr(i*int(w)/20), uintptr(h))
					gdi32.NewProc("MoveToEx").Call(hdc, 0, uintptr(i*int(h)/20), 0)
					gdi32.NewProc("LineTo").Call(hdc, uintptr(w), uintptr(i*int(h)/20))
				}
			} else { // 渐变
				for i := 0; i < 255; i++ {
					r := [4]int32{int32(i * int(w) / 255), 0, int32((i + 1) * int(w) / 255), int32(h)}
					c := uint32(i | (i << 8) | (i << 16))
					brush, _, _ := gdi32.NewProc("CreateSolidBrush").Call(uintptr(c))
					user32.NewProc("FillRect").Call(hdc, uintptr(unsafe.Pointer(&r)), brush)
					gdi32.NewProc("DeleteObject").Call(brush)
				}
			}
		}
		user32.NewProc("EndPaint").Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	}
	ret, _, _ := user32.NewProc("DefWindowProcW").Call(hwnd, uintptr(msg), wp, lp)
	return ret
}

func main() {
	// 检测 Windows 版本，XP (5.x) 不支持 DPI 感知
	ver, _, _ := kernel32.NewProc("GetVersion").Call()
	verMajor := byte(ver) & 0xFF
	// Vista = 6.0, XP = 5.1, 仅 Vista 及以上启用 DPI 感知
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

	user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(cls)), 0, 0x80000000|0x10000000, 0, 0, w, h, 0, 0, inst, 0)

	var m struct {
		h    uintptr
		m    uint32
		w, l uintptr
		t    uint32
		pt   struct{ x, y int32 }
	}
	for {
		r, _, _ := user32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if r == 0 {
			break
		}
		user32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&m)))
	}
}
