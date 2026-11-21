package main

import (
	"fmt"
	"os"
	"strings"

	"onescreen/internal/platform"
)

// 只支持中英两种，启动时按系统语言定一次，换语言重启生效。
// 命令行输出保持英文：终端按 GBK 解码 UTF-8 会乱码。

type textSet struct {
	switchTo          string // 会拼上端口号
	autoFollow        string // %s 是焦点来源的名字（Oink / Deskflow）
	autoFollowOffline string // 掉线时标题
	enable            string
	delay             string
	peerPort          string // 另一台机器接的那个口
	localPort         string
	autostart         string
	about             string
	aboutHelp         string
	aboutKeys         string
	quit              string
	warnDdc           string
	needPeer          string
	aboutTitle        string
	aboutBody         string // %s 配置文件路径、%s 日志路径、%s 提示
	openGitHub        string
	tooltip           string
	aboutSub          string
	portNameUnknown   string
	closeBtn          string
}

var textsZH = textSet{
	switchTo:          "切到另一台",
	autoFollow:        "随 %s 自动切屏",
	autoFollowOffline: "随 %s 自动切屏 (未连接)",
	enable:            "启用",
	delay:             "延迟",
	peerPort:          "另一台接的接口",
	localPort:         "本机口",
	autostart:         "开机自启",
	about:             "帮助/关于…",
	aboutHelp:         "双击托盘切换屏幕；右键设置另一台接的接口。",
	aboutKeys:         "切过去黑屏 = 接口选错了，在托盘换一个即可。",
	quit:              "退出",
	warnDdc:           "显示器没有响应 DDC/CI",
	needPeer:          "请先在「另一台接的接口」里选一次",
	aboutTitle:        "帮助/关于",
	aboutBody:         "配置文件：%s\n运行日志：%s\n\n© 2026 iroha3\nLicense AGPLv3\n\n%s",
	openGitHub:        "打开 GitHub",
	tooltip:           "双击切屏",
	aboutSub:          "",
	portNameUnknown:   "接口",
	closeBtn:          "关闭",
}

var textsEN = textSet{
	switchTo:          "Switch to other PC",
	autoFollow:        "Follow %s focus",
	autoFollowOffline: "Follow %s focus (disconnected)",
	enable:            "Enabled",
	delay:             "Delay",
	peerPort:          "Other PC's port",
	localPort:         "This port",
	autostart:         "Start with Windows",
	about:             "Help/About…",
	aboutHelp:         "Double-click the tray icon to switch. Right-click to set the other PC's port.",
	aboutKeys:         "Black screen after switching = wrong port, pick another one.",
	quit:              "Quit",
	warnDdc:           "Monitor isn't responding to DDC/CI",
	needPeer:          "Pick the other PC's port first",
	aboutTitle:        "Help/About",
	aboutBody:         "Config: %s\nLog: %s\n\n© 2026 iroha3\nLicense AGPLv3\n\n%s",
	openGitHub:        "Open GitHub",
	tooltip:           "double-click to switch",
	aboutSub:          "",
	portNameUnknown:   "Port",
	closeBtn:          "Close",
}

var uiLang *textSet

// portDisplayName 将 VCP 0x60 数值转换为人能看懂的标准接口名
func portDisplayName(v int) string {
	switch v {
	case 1:
		return "VGA (1)"
	case 3:
		return "DVI (3)"
	case 4:
		return "DVI 2 (4)"
	case 15:
		return "DP 1 (15)"
	case 16:
		return "DP 2 (16)"
	case 17:
		return "HDMI 1 (17)"
	case 18:
		return "HDMI 2 (18)"
	case 19:
		return "USB-C (19)"
	default:
		return fmt.Sprintf("%s (%d)", t().portNameUnknown, v)
	}
}

// portShortName 用于主菜单 "切到另一台 (HDMI 1)"
func portShortName(v int) string {
	switch v {
	case 1:
		return "VGA"
	case 3:
		return "DVI"
	case 4:
		return "DVI 2"
	case 15:
		return "DP 1"
	case 16:
		return "DP 2"
	case 17:
		return "HDMI 1"
	case 18:
		return "HDMI 2"
	case 19:
		return "USB-C"
	default:
		return fmt.Sprintf("%d", v)
	}
}

// t 懒初始化：不能放在包级变量初始化里，启动时按系统语言定一次。
func t() *textSet {
	if uiLang != nil {
		return uiLang
	}
	// 调试用：ONESCREEN_LANG=en 可以在不改系统语言的情况下看英文界面
	if v := os.Getenv("ONESCREEN_LANG"); v != "" {
		if strings.HasPrefix(strings.ToLower(v), "en") {
			uiLang = &textsEN
		} else {
			uiLang = &textsZH
		}
		return uiLang
	}
	if platform.GetSystemLanguage() == "zh" {
		uiLang = &textsZH
	} else {
		uiLang = &textsEN
	}
	return uiLang
}

// langName 给日志用。
func langName() string {
	if t() == &textsZH {
		return "zh"
	}
	return "en"
}
