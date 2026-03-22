package main

import (
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassEx  = user32.NewProc("RegisterClassExW")
	procCreateWindowEx   = user32.NewProc("CreateWindowExW")
	procDefWindowProc    = user32.NewProc("DefWindowProcW")
	procBeginPaint       = user32.NewProc("BeginPaint")
	procEndPaint         = user32.NewProc("EndPaint")
	procInvalidateRect   = user32.NewProc("InvalidateRect")
	procGetMessage       = user32.NewProc("GetMessageW")
	procDispatchMessage  = user32.NewProc("DispatchMessageW")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procFillRect         = user32.NewProc("FillRect")
	procCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
	procDeleteObject     = gdi32.NewProc("DeleteObject")
	procLoadCursor       = user32.NewProc("LoadCursorW")
	procShowCursor       = user32.NewProc("ShowCursor")

	colors = []uint32{0x0000FF, 0x00FF00, 0xFFFFFF, 0x000000, 0xFF0000, 0x00FFFF, 0xFF00FF}
	idx    = 0
)

// WNDCLASSEXW 结构体定义，必须严格匹配 Windows API
type WNDCLASSEXW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

func wndProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case 0x0001: // WM_CREATE
		procShowCursor.Call(0)
	case 0x0100, 0x0201: // 按键或左键
		idx++
		if idx >= len(colors) {
			syscall.Exit(0)
		}
		procInvalidateRect.Call(hwnd, 0, 1)
	case 0x0204, 0x0010: // 右键或关闭
		procShowCursor.Call(1)
		syscall.Exit(0)
	case 0x000F: // WM_PAINT
		var ps struct {
			hdc    uintptr
			fErase uint32
			rc     [4]int32
			res    [32]byte
		}
		hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		w, _, _ := procGetSystemMetrics.Call(0)
		h, _, _ := procGetSystemMetrics.Call(1)
		rect := [4]int32{0, 0, int32(w), int32(h)}
		brush, _, _ := procCreateSolidBrush.Call(uintptr(colors[idx]))
		procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rect)), brush)
		procDeleteObject.Call(brush)
		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	}
	ret, _, _ := procDefWindowProc.Call(hwnd, uintptr(msg), wp, lp)
	return ret
}

func main() {
	inst, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	clsName, _ := syscall.UTF16PtrFromString("XP_ST_CLS")
	hCursor, _, _ := procLoadCursor.Call(0, 32512) // IDC_ARROW

	var wc WNDCLASSEXW
	wc.Size = uint32(unsafe.Sizeof(wc))
	wc.Style = 0x0003 // CS_HREDRAW | CS_VREDRAW
	wc.WndProc = syscall.NewCallback(wndProc)
	wc.Instance = inst
	wc.Cursor = hCursor
	wc.ClassName = clsName

	res, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	if res == 0 {
		panic("RegisterClassExW failed: " + err.Error())
	}

	hwnd, _, err := procCreateWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(clsName)),
		uintptr(unsafe.Pointer(clsName)),
		0x80000000|0x10000000, // WS_POPUP | WS_VISIBLE
		0, 0, 1920, 1080,
		0, 0, inst, 0,
	)
	if hwnd == 0 {
		panic("CreateWindowExW failed: " + err.Error())
	}

	var m struct {
		h    uintptr
		m    uint32
		w, l uintptr
		t    uint32
		pt   struct{ x, y int32 }
	}
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if r == 0 {
			break
		}
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}
