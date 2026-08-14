//go:build windows && !xp

package main

import "syscall"

var kernel32Win = syscall.NewLazyDLL("kernel32.dll")

const executionStateKeepAwake = 0x80000000 | 0x00000001 | 0x00000002

func keepDisplayOn() {
	kernel32Win.NewProc("SetThreadExecutionState").Call(uintptr(executionStateKeepAwake))
}
