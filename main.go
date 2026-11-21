package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/energye/systray"

	"onescreen/internal/ddc"
	"onescreen/internal/oink"
	"onescreen/internal/platform"
)

const version = "0.1.0"
const repoURL = "https://github.com/iroha3/onescreen"

var delayChoices = []int{200, 300, 400, 600, 800, 1000}

type app struct {
	mu     sync.RWMutex
	cfg    Config
	log    *logFile
	sw     *Switcher
	cancel context.CancelFunc

	monitor string // 显示器友好名
	oinkDir string // 找到的 Oink 目录，空 = 没装

	mWarn      *systray.MenuItem
	mSwitch    *systray.MenuItem
	mAuto      *systray.MenuItem
	mAutoOn    *systray.MenuItem
	mDelay     *systray.MenuItem
	delayItem  map[int]*systray.MenuItem
	mPeer      *systray.MenuItem
	peerItems  map[int]*systray.MenuItem
	mAutoStart *systray.MenuItem
}

func (a *app) Config() Config {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg
}

func (a *app) setConfig(c Config) {
	a.mu.Lock()
	a.cfg = c
	a.mu.Unlock()
	saveConfig(c, a.log)
}

// ---------------------------------------------------------------- 启动

func main() {
	if len(os.Args) > 1 && (strings.HasPrefix(os.Args[1], "--ddc") || os.Args[1] == "--monitor-name" || os.Args[1] == "--about") {
		ddcCLI(os.Args[1:])
		return
	}
	if !platform.AcquireSingleInstance() {
		platform.MessageBox("OneScreen", "OneScreen 已经在运行了。")
		return
	}

	log := newLog(dataDir())
	a := &app{log: log}
	a.cfg = loadConfig(log)
	a.sw = NewSwitcher(a.Config, log, a.onDdcProblem)

	log.Printf("OneScreen %s 启动（界面语言 %s）", version, langName())
	a.prepare()

	systray.Run(a.onReady, a.onExit)
}

func (a *app) prepare() {
	cfg := a.Config()

	// 配置可能跟着可执行文件拷过来：靠机器名认出并对调两口
	if host, err := os.Hostname(); err == nil && cfg.Machine != "" && cfg.Machine != host {
		old := cfg.Machine
		cfg.LocalInput, cfg.PeerInput = cfg.PeerInput, cfg.LocalInput
		cfg.Machine = host
		a.setConfig(cfg)
		a.log.Printf("[config] 配置来自 %s，本机口与对端口已对调", old)
	}
	if cfg.Machine == "" {
		host, _ := os.Hostname()
		cfg.Machine = host
		a.setConfig(cfg)
	}

	a.oinkDir = cfg.OinkDir
	if a.oinkDir == "" {
		a.oinkDir = oink.FindInstall()
		if a.oinkDir != "" {
			cfg.OinkDir = a.oinkDir
			cfg.AgentPath = filepath.Join(a.oinkDir, "oink-agent.exe")
			a.setConfig(cfg)
		}
	}
	if a.oinkDir != "" {
		a.log.Printf("[oink] %s", a.oinkDir)
	} else {
		a.log.Printf("[oink] 没找到，只做手动切换")
	}

	if name := ddc.MonitorName(); name != "" {
		a.monitor = name
		a.log.Printf("[ddc] 显示器 %s", name)
	}

	// 本机口：自动读取
	if cfg.LocalInput == 0 {
		if cur, _, err := ddc.GetVCP(cfg.Monitor, ddc.VCPInputSource); err == nil {
			cfg.LocalInput = int(cur)
			a.setConfig(cfg)
			a.log.Printf("[ddc] 本机口 = %d", cur)
		} else {
			a.log.Printf("[ddc] 读不到本机口：%v", err)
		}
	}
	// 另一台接的接口：默认取候选最小值
	if cfg.PeerInput == 0 || cfg.PeerInput == cfg.LocalInput {
		if cands := a.candidates(); len(cands) > 0 {
			cfg.PeerInput = cands[0]
			a.setConfig(cfg)
			a.log.Printf("[ddc] 另一台接的接口默认 = %d", cfg.PeerInput)
		}
	}
}

func (a *app) candidates() []int {
	cfg := a.Config()
	var out []int
	for _, v := range cfg.InputsCache {
		if v != cfg.LocalInput {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		for _, v := range []int{15, 17, 18} {
			if v != cfg.LocalInput {
				out = append(out, v)
			}
		}
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- 托盘

func (a *app) onReady() {
	systray.SetIcon(platform.TrayIconBytes())
	systray.SetTooltip("OneScreen · " + t().tooltip)
	systray.SetOnDClick(func(m systray.IMenu) {
		if a.Config().PeerInput == 0 {
			go func() {
				time.Sleep(150 * time.Millisecond)
				_ = m.ShowMenu()
			}()
			return
		}
		go a.sw.SwitchToPeer()
	})

	a.mWarn = systray.AddMenuItem(t().warnDdc, "")
	a.mWarn.Disable()
	a.mWarn.Hide()

	a.mSwitch = systray.AddMenuItem(t().switchTo, "")

	// 自动切屏：常驻，未连上时禁用并提示
	a.mAuto = systray.AddMenuItem(fmt.Sprintf(t().autoFollowOffline, a.focusSourceName()), "")
	a.mAuto.Disable()
	a.mAutoOn = a.mAuto.AddSubMenuItemCheckbox(t().enable, "", a.Config().AutoSwitch)
	if a.Config().AutoSwitch {
		a.mAutoOn.Check()
	} else {
		a.mAutoOn.Uncheck()
	}
	a.sw.SetPaused(!a.Config().AutoSwitch)
	a.mDelay = a.mAuto.AddSubMenuItem(t().delay, "")
	a.delayItem = map[int]*systray.MenuItem{}
	for _, ms := range delayChoices {
		a.delayItem[ms] = a.mDelay.AddSubMenuItem(strconv.Itoa(ms)+" ms", "")
	}

	systray.AddSeparator()

	a.mPeer = systray.AddMenuItem(t().peerPort, "")
	a.peerItems = map[int]*systray.MenuItem{}
	for _, v := range a.candidates() {
		v := v
		item := a.mPeer.AddSubMenuItem(portDisplayName(v), "")
		a.peerItems[v] = item
		item.Click(func() { a.setPeer(v) })
	}

	systray.AddSeparator()
	a.mAutoStart = systray.AddMenuItemCheckbox(t().autostart, "", platform.IsAutostartEnabled())
	mAbout := systray.AddMenuItem(t().about, "")
	mQuit := systray.AddMenuItem(t().quit, "")

	a.refreshLabels()

	go a.refreshInputsCache()

	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		a.cancel = cancel
		platform.StartEmergencyHotkey(ctx, a.sw.SwitchToLocal, a.log.Printf)
		go oink.RunEvents(ctx, a.Config().OinkDir, a.log.Printf,
			func(f oink.Focus) {
				a.sw.OnFocus(f)
			},
			func() {
				a.setOinkOnline(true)
				a.sw.Resync()
			},
			func(msg string) {
				if msg == "" {
					a.setOinkOnline(true)
					return
				}
				a.setOinkOnline(false)
			})
	}()

	a.mSwitch.Click(func() { a.sw.SwitchToPeer() })
	a.mAutoOn.Click(func() { a.toggleAuto() })
	a.mAutoStart.Click(func() { a.toggleAutostart() })
	mAbout.Click(func() {
		if !platform.ShowAbout(version, repoURL, aboutContent(), t().aboutTitle, t().openGitHub) {
			platform.MessageBox(t().aboutTitle, "OneScreen "+version+"\n\n"+aboutContent())
		}
	})
	mQuit.Click(func() {
		a.log.Printf("退出")
		systray.Quit()
	})

	for ms, item := range a.delayItem {
		ms, item := ms, item
		item.Click(func() { a.setDelay(ms, item) })
	}
}

func (a *app) focusSourceName() string {
	switch a.Config().FocusSource {
	case "", "oink":
		return "Oink"
	default:
		return a.Config().FocusSource
	}
}

func (a *app) setOinkOnline(online bool) {
	if a.mAuto == nil {
		return
	}
	if online {
		a.mAuto.Enable()
		a.mAuto.SetTitle(fmt.Sprintf(t().autoFollow, a.focusSourceName()))
	} else {
		a.mAuto.Disable()
		a.mAuto.SetTitle(fmt.Sprintf(t().autoFollowOffline, a.focusSourceName()))
	}
}

func (a *app) onExit() {
	if a.cancel != nil {
		a.cancel()
	}
	a.log.Printf("已退出")
}

// ---------------------------------------------------------------- 菜单状态

func (a *app) refreshLabels() {
	cfg := a.Config()
	if a.mSwitch != nil {
		if cfg.PeerInput > 0 {
			a.mSwitch.SetTitle(fmt.Sprintf("%s (%s)", t().switchTo, portShortName(cfg.PeerInput)))
		} else {
			a.mSwitch.SetTitle(t().switchTo)
		}
	}
	if a.mPeer != nil {
		a.mPeer.SetTitle(t().peerPort)
		for v, item := range a.peerItems {
			if v == cfg.PeerInput {
				item.Check()
			} else {
				item.Uncheck()
			}
		}
	}
	if a.mDelay != nil {
		a.mDelay.SetTitle(fmt.Sprintf("%s %dms", t().delay, cfg.DelayMs))
		for ms, item := range a.delayItem {
			if ms == cfg.DelayMs {
				item.Check()
			} else {
				item.Uncheck()
			}
		}
	}
}

func (a *app) setPeer(v int) {
	c := a.Config()
	c.PeerInput = v
	a.setConfig(c)
	a.refreshLabels()
	a.log.Printf("[config] 另一台接的接口 = %d", v)
}

func (a *app) toggleAuto() {
	enable := !a.mAutoOn.Checked()
	if enable {
		a.mAutoOn.Check()
	} else {
		a.mAutoOn.Uncheck()
	}
	c := a.Config()
	c.AutoSwitch = enable
	a.setConfig(c)
	a.sw.SetPaused(!enable)
}

func (a *app) setDelay(ms int, item *systray.MenuItem) {
	c := a.Config()
	c.DelayMs = ms
	a.setConfig(c)
	for _, it := range a.delayItem {
		it.Uncheck()
	}
	item.Check()
	a.mDelay.SetTitle(fmt.Sprintf("%s %dms", t().delay, ms))
	a.log.Printf("[config] 延迟 = %dms", ms)
}

func (a *app) toggleAutostart() {
	enable := !a.mAutoStart.Checked()
	if err := platform.SetAutostart(enable); err != nil {
		a.log.Printf("[config] 设置开机自启失败：%v", err)
		return
	}
	if enable {
		a.mAutoStart.Check()
	} else {
		a.mAutoStart.Uncheck()
	}
	c := a.Config()
	c.Autostart = enable
	a.setConfig(c)
}

func (a *app) onDdcProblem(msg string) {
	if a.mWarn == nil {
		return
	}
	if msg == "" {
		a.mWarn.Hide()
		return
	}
	a.mWarn.SetTitle(msg)
	a.mWarn.Show()
}

func (a *app) refreshInputsCache() {
	c := a.Config()
	if len(c.InputsCache) > 0 {
		return
	}
	time.Sleep(5 * time.Second)
	found, err := ddc.Inputs(c.Monitor)
	if err != nil || len(found) == 0 {
		a.log.Printf("[ddc] 读不到输入源列表：%v", err)
		return
	}
	vals := make([]int, len(found))
	for i, v := range found {
		vals[i] = int(v)
	}
	if sameInts(vals, c.InputsCache) {
		return
	}
	c.InputsCache = vals
	a.setConfig(c)
	a.log.Printf("[ddc] 显示器支持的输入源：%v（下次启动生效）", vals)
}

func sameInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func aboutContent() string {
	return fmt.Sprintf(t().aboutBody, configPath(), logPath(), t().aboutKeys)
}

// ---------------------------------------------------------------- 命令行

func ddcCLI(args []string) {
	platform.AttachConsole()
	sel := "0"
	if len(args) > 1 {
		sel = args[1]
	}
	switch args[0] {
	case "--ddc-list":
		ms, err := ddc.EnumMonitors()
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		defer ddc.CloseMonitors(ms)
		inputs, _ := ddc.Inputs(sel)
		for i, m := range ms {
			line := fmt.Sprintf("%d  %s", i, m.Desc)
			if len(inputs) > 0 {
				parts := make([]string, len(inputs))
				for j, v := range inputs {
					parts[j] = strconv.Itoa(int(v))
				}
				line += "   inputs: " + strings.Join(parts, " ")
			}
			fmt.Println(line)
		}
	case "--monitor-name":
		fmt.Println(ddc.MonitorName())
	case "--about":
		if !platform.ShowAbout(version, repoURL, aboutContent(), t().aboutTitle, t().openGitHub) {
			platform.MessageBox(t().aboutTitle, "OneScreen "+version+"\n\n"+aboutContent())
		}
	case "--ddc-caps":
		caps, err := ddc.Caps(sel)
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		fmt.Println(caps)
	case "--ddc-get":
		code := byte(ddc.VCPInputSource)
		if len(args) > 2 {
			var c uint32
			fmt.Sscanf(args[2], "%v", &c)
			code = byte(c)
		}
		cur, max, err := ddc.GetVCP(sel, code)
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		fmt.Printf("VCP 0x%02X = %d (max %d)\n", code, cur, max)
	case "--ddc-set":
		if len(args) < 3 {
			fmt.Println("usage: --ddc-set <monitor> <value> [code]")
			return
		}
		var v uint32
		fmt.Sscanf(args[2], "%d", &v)
		code := byte(ddc.VCPInputSource)
		if len(args) > 3 {
			var c uint32
			fmt.Sscanf(args[3], "%v", &c)
			code = byte(c)
		}
		if err := ddc.SetVCP(sel, code, v); err != nil {
			fmt.Println("error:", err)
			return
		}
		fmt.Printf("wrote VCP 0x%02X = %d\n", code, v)
	default:
		fmt.Println("usage: --ddc-list | --ddc-caps [monitor] | --ddc-get [monitor] [code] | --ddc-set <monitor> <value> [code] | --monitor-name")
	}
}
