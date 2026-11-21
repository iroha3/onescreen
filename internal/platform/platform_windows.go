//go:build windows

package platform

import (
	"context"
	_ "embed"
	"encoding/binary"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

//go:embed tray.ico
var trayIconBytes []byte

// TrayIconBytes 返回当前平台托盘所需的图标字节（Windows 为 ICO）
func TrayIconBytes() []byte {
	return trayIconBytes
}

var (
	user32                       = syscall.NewLazyDLL("user32.dll")
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	shell32                      = syscall.NewLazyDLL("shell32.dll")
	comctl32                     = syscall.NewLazyDLL("comctl32.dll")
	procAttachConsole            = kernel32.NewProc("AttachConsole")
	procGetStdHandle             = kernel32.NewProc("GetStdHandle")
	procMessageBox               = user32.NewProc("MessageBoxW")
	procShellExecute             = shell32.NewProc("ShellExecuteW")
	procRegisterHotKey           = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey         = user32.NewProc("UnregisterHotKey")
	procGetMessage               = user32.NewProc("GetMessageW")
	procCreateMutex              = kernel32.NewProc("CreateMutexW")
	procTaskDialogIndirect       = comctl32.NewProc("TaskDialogIndirect")
	procGetUserDefaultUILanguage = kernel32.NewProc("GetUserDefaultUILanguage")
)

// AttachConsole 让 GUI 子系统程序也能向调用方的控制台打印输出。
func AttachConsole() {
	const stdOutputHandle = ^uintptr(11) // -11
	if h, _, _ := procGetStdHandle.Call(stdOutputHandle); h != 0 && h != ^uintptr(0) {
		return
	}
	const attachParentProcess = ^uintptr(0) // (DWORD)-1
	if r, _, _ := procAttachConsole.Call(attachParentProcess); r == 0 {
		return
	}
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout = f
		os.Stderr = f
	}
}

// AcquireSingleInstance 用命名互斥体保证只有一个实例在跑。
func AcquireSingleInstance() bool {
	name, err := syscall.UTF16PtrFromString(`Local\OneScreen.SingleInstance`)
	if err != nil {
		return true
	}
	h, _, callErr := procCreateMutex.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return true
	}
	const errorAlreadyExists = syscall.Errno(183)
	return callErr != errorAlreadyExists
}

// GetSystemLanguage 获取系统界面语言代码（"zh" 或 "en"）
func GetSystemLanguage() string {
	r, _, _ := procGetUserDefaultUILanguage.Call()
	if uint16(r)&0x3ff == 0x04 {
		return "zh"
	}
	return "en"
}

// MessageBox 弹出标准原生模态对话框
func MessageBox(title, body string) {
	pt, err1 := syscall.UTF16PtrFromString(title)
	pb, err2 := syscall.UTF16PtrFromString(body)
	if err1 != nil || err2 != nil {
		return
	}
	const mbOK = 0x0
	procMessageBox.Call(0, uintptr(unsafe.Pointer(pb)), uintptr(unsafe.Pointer(pt)), mbOK)
}

// OpenURL 调用系统默认浏览器打开 URL
func OpenURL(url string) {
	verb, _ := syscall.UTF16PtrFromString("open")
	u, err := syscall.UTF16PtrFromString(url)
	if err != nil {
		return
	}
	procShellExecute.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(u)), 0, 0, 1)
}

const (
	tdfAllowDialogCancellation = 0x0008
	tdfSizeToContent           = 0x01000000
	tdcbfCloseButton           = 0x0020
	aboutButtonGitHub          = 101
)

// ShowAbout 弹出关于对话框，支持点击直接打开 GitHub。
func ShowAbout(version, repoURL, content, title, openGitHubText string) bool {
	defer func() {
		_ = recover()
	}()

	if err := procTaskDialogIndirect.Find(); err != nil {
		return false
	}

	pTitle, e1 := syscall.UTF16PtrFromString(title)
	pInst, e2 := syscall.UTF16PtrFromString("OneScreen " + version)
	pBody, e3 := syscall.UTF16PtrFromString(content)
	pGh, e4 := syscall.UTF16PtrFromString(openGitHubText)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
		return false
	}

	// 按钮：pack(4) 下为 nButtonID(4) + pszButtonText(8) = 12 字节
	var btnBuf [12]byte
	binary.LittleEndian.PutUint32(btnBuf[0:4], aboutButtonGitHub)
	*(*uintptr)(unsafe.Pointer(&btnBuf[4])) = uintptr(unsafe.Pointer(pGh))

	// TASKDIALOGCONFIG pack(4) 160 字节精准布局
	var buf [160]byte
	binary.LittleEndian.PutUint32(buf[0:4], 160)
	binary.LittleEndian.PutUint32(buf[20:24], tdfAllowDialogCancellation|tdfSizeToContent)
	binary.LittleEndian.PutUint32(buf[24:28], tdcbfCloseButton)
	*(*uintptr)(unsafe.Pointer(&buf[28])) = uintptr(unsafe.Pointer(pTitle))
	*(*uintptr)(unsafe.Pointer(&buf[44])) = uintptr(unsafe.Pointer(pInst))
	*(*uintptr)(unsafe.Pointer(&buf[52])) = uintptr(unsafe.Pointer(pBody))
	binary.LittleEndian.PutUint32(buf[60:64], 1)
	*(*uintptr)(unsafe.Pointer(&buf[64])) = uintptr(unsafe.Pointer(&btnBuf[0]))

	var pressed int32
	r, _, _ := procTaskDialogIndirect.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&pressed)),
		0, 0,
	)
	if r != 0 {
		return false
	}
	if pressed == aboutButtonGitHub {
		OpenURL(repoURL)
	}
	return true
}

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const runValue = "OneScreen"

func IsAutostartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(runValue)
	return err == nil
}

func SetAutostart(enable bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !enable {
		return k.DeleteValue(runValue)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return k.SetStringValue(runValue, exe)
}

const (
	modAlt      = 0x0001
	modControl  = 0x0002
	modShift    = 0x0004
	modNoRepeat = 0x4000
	wmHotkey    = 0x0312
	hotkeyID    = 0x5343 // 'SC' Screen
)

type winMsg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

// StartEmergencyHotkey 注册全局紧急逃生热键（Ctrl+Alt+Shift+S）
func StartEmergencyHotkey(ctx context.Context, onTrigger func(), log func(format string, args ...any)) {
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		mods := uintptr(modControl | modAlt | modShift | modNoRepeat)
		vk := uintptr('S')
		r, _, _ := procRegisterHotKey.Call(0, uintptr(hotkeyID), mods, vk)
		if r == 0 {
			if log != nil {
				log("[hotkey] 注册应急热键失败（可能已被占用）")
			}
			return
		}
		defer procUnregisterHotKey.Call(0, uintptr(hotkeyID))
		if log != nil {
			log("[hotkey] 应急逃生热键已就绪：Ctrl+Alt+Shift+S（强制切回本机）")
		}

		var m winMsg
		for ctx.Err() == nil {
			ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(ret) <= 0 {
				break
			}
			if m.message == wmHotkey && m.wParam == uintptr(hotkeyID) {
				if log != nil {
					log("[hotkey] 触发应急热键：强制切回本机")
				}
				if onTrigger != nil {
					go onTrigger()
				}
			}
		}
	}()
}

// HideWindow 抑制后台子进程弹出黑框命令行
func HideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
