//go:build linux

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

// TrayIconBytes 返回当前平台托盘所需的图标字节（Linux 为 PNG）
func TrayIconBytes() []byte {
	return trayIconBytes
}

var lockFile *os.File

func AttachConsole() {}

// AcquireSingleInstance 用文件锁保证只有一个实例在跑。
func AcquireSingleInstance() bool {
	dir := os.Getenv("XDG_RUNTIME_DIR")
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
	return "en"
}

func ensureLinuxIconFile() string {
	p := "/tmp/onescreen_icon.png"
	if len(trayIconBytes) > 0 {
		_ = os.WriteFile(p, trayIconBytes, 0o644)
		return p
	}
	return "help-about"
}

// ShowAbout 弹出关于对话框，通过注入自定义图标消除黄色感叹号，并添加打开 GitHub 按钮。
func ShowAbout(version, repoURL, content, title, openGitHubText string) bool {
	iconPath := ensureLinuxIconFile()

	closeLabel := "关闭"
	if GetSystemLanguage() != "zh" {
		closeLabel = "Close"
	}

	if _, err := exec.LookPath("zenity"); err == nil {
		go func() {
			text := fmt.Sprintf("<big><b>OneScreen %s</b></big>\n\n%s\n\n<a href=\"%s\">%s</a>",
				version, content, repoURL, repoURL)
			cmd := exec.Command("zenity", "--info",
				"--title="+title,
				"--text="+text,
				"--icon-name="+iconPath,
				"--window-icon="+iconPath,
				"--ok-label="+closeLabel,
				"--extra-button="+openGitHubText,
				"--no-wrap",
			)
			out, _ := cmd.Output()
			if strings.Contains(string(out), openGitHubText) {
				OpenURL(repoURL)
			}
		}()
		return true
	}

	if _, err := exec.LookPath("kdialog"); err == nil {
		go func() {
			text := fmt.Sprintf("OneScreen %s\n\n%s\n\n%s", version, content, repoURL)
			cmd := exec.Command("kdialog",
				"--title", title,
				"--icon", iconPath,
				"--yes-label", openGitHubText,
				"--no-label", closeLabel,
				"--yesno", text,
			)
			if cmd.Run() == nil {
				OpenURL(repoURL)
			}
		}()
		return true
	}

	MessageBox(title, "OneScreen "+version+"\n\n"+content+"\n\n"+repoURL)
	return true
}

func MessageBox(title, body string) {
	iconPath := ensureLinuxIconFile()
	if _, err := exec.LookPath("zenity"); err == nil {
		_ = exec.Command("zenity", "--info",
			"--title="+title,
			"--text="+body,
			"--icon-name="+iconPath,
			"--window-icon="+iconPath,
			"--no-wrap",
		).Start()
		return
	}
	if _, err := exec.LookPath("kdialog"); err == nil {
		_ = exec.Command("kdialog", "--title", title, "--icon", iconPath, "--msgbox", body).Start()
		return
	}
	fmt.Printf("[%s]\n%s\n", title, body)
}

func OpenURL(url string) {
	_ = exec.Command("xdg-open", url).Start()
}

func autostartDesktopPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "autostart", "onescreen.desktop")
}

func IsAutostartEnabled() bool {
	_, err := os.Stat(autostartDesktopPath())
	return err == nil
}

func SetAutostart(enable bool) error {
	p := autostartDesktopPath()
	if !enable {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	content := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=OneScreen\nExec=%s\nHidden=false\nNoDisplay=false\nX-GNOME-Autostart-enabled=true\n", exe)
	return os.WriteFile(p, []byte(content), 0o644)
}

func StartEmergencyHotkey(ctx context.Context, onTrigger func(), log func(format string, args ...any)) {
	if log != nil {
		log("[hotkey] Linux 环境下请通过系统快捷键绑定 onescreen --switch-local 应急切回")
	}
}

func HideWindow(cmd *exec.Cmd) {}
