//go:build darwin

package platform

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

//go:embed tray.png
var trayIconBytes []byte

// TrayIconBytes 返回当前平台托盘所需的图标字节（macOS 为 PNG）
func TrayIconBytes() []byte {
	return trayIconBytes
}

var lockFile *os.File

func AttachConsole() {}

// AcquireSingleInstance 用文件锁保证只有一个实例在跑。
func AcquireSingleInstance() bool {
	dir := os.Getenv("TMPDIR")
	if dir == "" {
		dir = "/tmp"
	}
	p := filepath.Join(dir, "onescreen.lock")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return true
	}
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		_ = f.Close()
		return false
	}
	lockFile = f
	return true
}

func GetSystemLanguage() string {
	lang := strings.ToLower(os.Getenv("LANG"))
	if strings.HasPrefix(lang, "zh") {
		return "zh"
	}
	// 通过 defaults read -g AppleLanguages 查询首选语言列表
	out, err := exec.Command("defaults", "read", "-g", "AppleLanguages").Output()
	if err == nil && strings.Contains(strings.ToLower(string(out)), "zh") {
		return "zh"
	}
	return "en"
}

// ShowAbout 弹出关于对话框，支持点击直接打开 GitHub。
func ShowAbout(version, repoURL, content, title, openGitHubText string) bool {
	closeLabel := "关闭"
	if GetSystemLanguage() != "zh" {
		closeLabel = "Close"
	}

	go func() {
		msg := fmt.Sprintf("OneScreen %s\n\n%s", version, content)
		script := fmt.Sprintf(`
			set res to display alert "%s" message "%s" buttons {"%s", "%s"} default button "%s"
			if button returned of res is "%s" then
				open location "%s"
			end if
		`, escapeAppleScript(title), escapeAppleScript(msg), escapeAppleScript(closeLabel), escapeAppleScript(openGitHubText), escapeAppleScript(openGitHubText), escapeAppleScript(openGitHubText), repoURL)

		cmd := exec.Command("osascript", "-e", script)
		_ = cmd.Run()
	}()
	return true
}

func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func MessageBox(title, body string) {
	script := fmt.Sprintf(`display alert "%s" message "%s" buttons {"OK"} default button "OK"`,
		escapeAppleScript(title), escapeAppleScript(body))
	_ = exec.Command("osascript", "-e", script).Start()
}

func OpenURL(url string) {
	_ = exec.Command("open", url).Start()
}

func launchAgentPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "com.iroha3.onescreen.plist")
}

func IsAutostartEnabled() bool {
	p := launchAgentPath()
	_, err := os.Stat(p)
	return err == nil
}

func SetAutostart(enable bool) error {
	p := launchAgentPath()
	if !enable {
		_ = os.Remove(p)
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.iroha3.onescreen</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <false/>
</dict>
</plist>`, exe)
	return os.WriteFile(p, []byte(plist), 0o644)
}

func StartEmergencyHotkey(ctx context.Context, onTrigger func(), log func(format string, args ...any)) {
	if log != nil {
		log("[hotkey] macOS 环境下应急逃生建议使用托盘双击切回，或由快捷指令触发")
	}
}

func HideWindow(cmd *exec.Cmd) {}
