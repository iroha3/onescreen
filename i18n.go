package main

import (
	"os"
	"strings"
)

// 只支持中英两种，启动时按系统语言定一次，换语言重启生效。
// 命令行输出保持英文：终端按 GBK 解码 UTF-8 会乱码。

type textSet struct {
	switchTo   string // 会拼上端口号
	autoFollow string // %s 是焦点来源的名字（Oink / Deskflow）
	enable     string
	delay      string
	peerPort   string
	localPort  string
	autostart  string
	about      string
	quit       string
	warnDdc    string
	needPeer   string
	aboutTitle string
	aboutSub   string
}

var textsZH = textSet{
	switchTo:   "切到另一台",
	autoFollow: "随 %s 自动切屏",
	enable:     "启用",
	delay:      "延迟",
	peerPort:   "对端口",
	localPort:  "本机口",
	autostart:  "开机自启",
	about:      "关于",
	quit:       "退出",
	warnDdc:    "显示器没有响应 DDC/CI",
	needPeer:   "请先在「对端口」里选一次",
	aboutTitle: "关于 OneScreen",
	aboutSub:   "单显示器多设备切换",
}

var textsEN = textSet{
	switchTo:   "Switch to PC",
	autoFollow: "Follow %s focus",
	enable:     "Enabled",
	delay:      "Delay",
	peerPort:   "Peer port",
	localPort:  "This port",
	autostart:  "Start with Windows",
	about:      "About",
	quit:       "Quit",
	warnDdc:    "Monitor isn't responding to DDC/CI",
	needPeer:   "Pick a peer port first",
	aboutTitle: "About OneScreen",
	aboutSub:   "One screen, many computers",
}

var uiLang *textSet

// t 懒初始化：不能放在包级变量初始化里，那时 kernel32 可能还没就绪。
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
	proc := kernel32.NewProc("GetUserDefaultUILanguage")
	r, _, _ := proc.Call()
	// LANGID 低 10 位是主语言，0x04 = 中文
	if uint16(r)&0x3ff == 0x04 {
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
