package main

import (
	_ "embed"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/energye/systray"
	"golang.org/x/sys/windows/registry"
)

//go:embed tray.ico
var iconBytes []byte

const version = "0.1.0"
const copyright = "© 2026"

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const runValue = "OneScreen"

var delayChoices = []int{200, 300, 400, 600, 800, 1000}

var (
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procAttachConsole = kernel32.NewProc("AttachConsole")
	procGetStdHandle  = kernel32.NewProc("GetStdHandle")
	procMessageBox    = user32.NewProc("MessageBoxW")
)

// attachConsole 让 GUI 子系统程序也能往调用方的控制台打印。
func attachConsole() {
	const stdOutputHandle = ^uintptr(11) // -11
	if h, _, _ := procGetStdHandle.Call(stdOutputHandle); h != 0 && h != ^uintptr(0) {
		return // 已经有可用的 stdout（管道、重定向、终端里直接跑），别动它
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
	if len(os.Args) > 1 && (strings.HasPrefix(os.Args[1], "--ddc") || os.Args[1] == "--monitor-name") {
		ddcCLI(os.Args[1:])
		return
	}
	if !acquireSingleInstance() {
		messageBox("OneScreen", "OneScreen 已经在运行了。")
		return
	}

	log := newLog(dataDir())
	a := &app{log: log}
	currentApp = a
	a.cfg = loadConfig(log)
	a.sw = NewSwitcher(a.Config, log, a.onDdcProblem)

	log.Printf("OneScreen %s 启动", version)
	a.prepare()

	systray.Run(a.onReady, a.onExit)
}

// prepare 把「这台机器」的事实确定下来：本机口、Oink 在不在、显示器叫什么。
func (a *app) prepare() {
	cfg := a.Config()

	// 配置可能跟着 exe 从另一台机器拷过来：靠机器名认出来，两个口对调。
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
		a.oinkDir = findOinkInstall()
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

	if name := monitorName(); name != "" {
		a.monitor = name
		a.log.Printf("[ddc] 显示器 %s", name)
	}

	// 本机口：只读，自动读一次。你能点到托盘，说明屏幕正显示这台机器。
	if cfg.LocalInput == 0 {
		if cur, _, err := ddcGet(cfg.Monitor, vcpInputSource); err == nil {
			cfg.LocalInput = int(cur)
			a.setConfig(cfg)
			a.log.Printf("[ddc] 本机口 = %d", cur)
		} else {
			a.log.Printf("[ddc] 读不到本机口：%v", err)
		}
	}
	// 对端口不猜：留空，等用户从菜单里选一次。
}

// candidates 是对端口可选项：显示器支持的输入源里排除本机口。
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
	systray.SetIcon(iconBytes)
	systray.SetTooltip("OneScreen")
	systray.SetOnDClick(func(m systray.IMenu) {
		if a.Config().PeerInput == 0 {
			// 还没选过对端口：把菜单弹出来，让双击这个动作直接带到设置项前面
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

	// 自动切屏：只在 Oink 连上时显示
	a.mAuto = systray.AddMenuItem(fmt.Sprintf(t().autoFollow, a.focusSourceName()), "")
	a.mAutoOn = a.mAuto.AddSubMenuItemCheckbox(t().enable, "", a.Config().AutoSwitch)
	a.mDelay = a.mAuto.AddSubMenuItem(t().delay, "")
	a.delayItem = map[int]*systray.MenuItem{}
	for _, ms := range delayChoices {
		a.delayItem[ms] = a.mDelay.AddSubMenuItem(strconv.Itoa(ms)+" ms", "")
	}
	a.mAuto.Hide()

	systray.AddSeparator()

	a.mPeer = systray.AddMenuItem(t().peerPort, "")
	a.peerItems = map[int]*systray.MenuItem{}
	for _, v := range a.candidates() {
		v := v
		item := a.mPeer.AddSubMenuItem(strconv.Itoa(v), "")
		a.peerItems[v] = item
		item.Click(func() { a.setPeer(v) })
	}

	systray.AddSeparator()
	a.mAutoStart = systray.AddMenuItemCheckbox(t().autostart, "", isAutostartEnabled())
	mAbout := systray.AddMenuItem(t().about, "")
	mQuit := systray.AddMenuItem(t().quit, "")

	a.refreshLabels()

	go a.refreshInputsCache()

	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		a.cancel = cancel
		go runEvents(ctx, a.Config, a.log,
			func(f Focus) {
				a.sw.OnFocus(f)
				systray.SetTooltip("OneScreen · " + a.sw.StatusText())
			},
			func() {
				a.setOinkOnline(true)
				a.sw.Resync()
				systray.SetTooltip("OneScreen · " + a.sw.StatusText())
			},
			func(msg string) {
				if msg == "" {
					a.setOinkOnline(true)
					return
				}
				a.setOinkOnline(false)
				a.log.Printf("[oink] %s", msg)
			})
	}()

	a.mSwitch.Click(func() { a.sw.SwitchToPeer() })
	a.mAutoOn.Click(func() { a.toggleAuto() })
	a.mAutoStart.Click(func() { a.toggleAutostart() })
	mAbout.Click(showAbout)
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

// setOinkOnline 控制自动切屏那一组显不显示。没连上就没有这一组。
func (a *app) setOinkOnline(online bool) {
	if a.mAuto == nil {
		return
	}
	if online {
		a.mAuto.Show()
	} else {
		a.mAuto.Hide()
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
			a.mSwitch.SetTitle(fmt.Sprintf("%s (%d)", t().switchTo, cfg.PeerInput))
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
	a.log.Printf("[config] 对端口 = %d", v)
}

func (a *app) toggleAuto() {
	c := a.Config()
	c.AutoSwitch = a.mAutoOn.Checked()
	a.setConfig(c)
	a.sw.SetPaused(!c.AutoSwitch)
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
	if err := setAutostart(enable); err != nil {
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

// onDdcProblem 显示器不响应时把警告行亮出来，正常了再藏起来。
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

// refreshInputsCache 后台读一次能力串，把支持的输入源存进配置。
func (a *app) refreshInputsCache() {
	c := a.Config()
	if len(c.InputsCache) > 0 {
		return // 已经缓存过，读一次要 1.8 秒，不再重复打扰显示器
	}
	// 读能力串会占住 DDC 通道 1.8 秒，等启动时的状态对齐先做完
	time.Sleep(5 * time.Second)
	found, err := ddcInputs(c.Monitor)
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

// ---------------------------------------------------------------- 关于

var currentApp *app

func showAbout() {
	a := currentApp
	cfg := a.Config()
	monitor := a.monitor
	if monitor == "" {
		monitor = "?"
	}
	body := fmt.Sprintf(
		"OneScreen %s\n%s\n\n%s %d · %s\n%s\n%s\n\n%s",
		version, t().aboutSub,
		t().localPort, cfg.LocalInput, monitor,
		configPath(),
		filepath.Join(dataDir(), "switch.log"),
		copyright,
	)
	messageBox(t().aboutTitle, body)
}

func messageBox(title, body string) {
	pt, err1 := syscall.UTF16PtrFromString(title)
	pb, err2 := syscall.UTF16PtrFromString(body)
	if err1 != nil || err2 != nil {
		return
	}
	const mbOK = 0x0
	procMessageBox.Call(0, uintptr(unsafe.Pointer(pb)), uintptr(unsafe.Pointer(pt)), mbOK)
}

// ---------------------------------------------------------------- 开机自启

func isAutostartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(runValue)
	return err == nil
}

func setAutostart(enable bool) error {
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

// ---------------------------------------------------------------- 命令行

func ddcCLI(args []string) {
	attachConsole()
	sel := "0"
	if len(args) > 1 {
		sel = args[1]
	}
	switch args[0] {
	case "--ddc-list":
		ms, err := enumMonitors()
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		defer closeMonitors(ms)
		inputs, _ := ddcInputs(sel)
		for i, m := range ms {
			line := fmt.Sprintf("%d  %s", i, m.desc)
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
		fmt.Println(monitorName())
	case "--ddc-caps":
		caps, err := ddcCaps(sel)
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		fmt.Println(caps)
	case "--ddc-get":
		code := byte(vcpInputSource)
		if len(args) > 2 {
			var c uint32
			fmt.Sscanf(args[2], "%v", &c)
			code = byte(c)
		}
		cur, max, err := ddcGet(sel, code)
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
		code := byte(vcpInputSource)
		if len(args) > 3 {
			var c uint32
			fmt.Sscanf(args[3], "%v", &c)
			code = byte(c)
		}
		if err := ddcSet(sel, code, v); err != nil {
			fmt.Println("error:", err)
			return
		}
		fmt.Printf("wrote VCP 0x%02X = %d\n", code, v)
	default:
		fmt.Println("usage: --ddc-list | --ddc-caps [monitor] | --ddc-get [monitor] [code] | --ddc-set <monitor> <value> [code] | --monitor-name")
	}
}
