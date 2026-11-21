package main

import (
	"syscall"
	"unsafe"
)

var procCreateMutex = kernel32.NewProc("CreateMutexW")

// acquireSingleInstance 用命名互斥体保证只有一个实例在跑。
// 两个实例同时写 DDC 会互相打架，所以这一步在启动最前面。
// 返回 false 表示已经有实例在运行。
func acquireSingleInstance() bool {
	name, err := syscall.UTF16PtrFromString(`Local\OneScreen.SingleInstance`)
	if err != nil {
		return true
	}
	h, _, callErr := procCreateMutex.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return true // 创建失败就放行，不要因为这个把用户挡在门外
	}
	// 句柄不关，进程退出时系统自动释放
	const errorAlreadyExists = syscall.Errno(183)
	if callErr == errorAlreadyExists {
		return false
	}
	return true
}
