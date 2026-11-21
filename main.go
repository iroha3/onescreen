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

const repoURL = "https://github.com/"

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
	oinkOK  bool

	mWarn      *systray.MenuItem
	mSwitch    *systray.MenuItem
	mAuto      *systray.MenuItem
	mDelay     *systray.MenuItem
	delayItem  map[int]*systray.MenuItem
	mLocalInfo *systray.MenuItem
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
	log := newLog(dataDir())
	a := &app{log: log}
	a.cfg = loadConfig(log)
	a.sw = NewSwitcher(a.Config, log, a.onDdcProblem)

	log.Printf("OneScreen %s 启动", version)
	a.prepare()

	systray.Run(a.onReady, a.onExit)
}

// prepare 把「这台机器」的事实确定下来：本机口、对端口、Oink 在不在。
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

	if cfg.OinkDir == "" {
		cfg.OinkDir = findOinkInstall()
		if cfg.OinkDir != "" {
			cfg.AgentPath = filepath.Join(cfg.OinkDir, "oink-agent.exe")
			a.setConfig(cfg)
		}
	}
	a.oinkOK = cfg.OinkDir != ""
	a.log.Printf("[oink] %s", map[bool]string{true: "找到 " + cfg.OinkDir, false: "没装，只做手动切换"}[a.oinkOK])

	a.monitor = monitorName()
	if a.monitor != "" {
		a.log.Printf("[ddc] 显示器 %s", a.monitor)
	}

	// 本机口：只读，自动读一次
	if cfg.LocalInput == 0 {
		if cur, _, err := ddcGet(cfg.Monitor, vcpInputSource); err == nil {
			cfg.LocalInput = int(cur)
			a.setConfig(cfg)
			a.log.Printf("[ddc] 本机口 = %d", cur)
		} else {
			a.log.Printf("[ddc] 读不到本机口：%v", err)
		}
	}

	// 对端口：从候选里默认取最小的那个
	if cfg.PeerInput == 0 || cfg.PeerInput == cfg.LocalInput {
		if cands := a.candidates(); len(cands) > 0 {
			cfg.PeerInput = cands[0]
			a.setConfig(cfg)
			a.log.Printf("[ddc] 对端口默认 = %d（可在托盘里改）", cfg.PeerInput)
		}
	}
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
	// 从小到大，默认取第一个
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
	systray.SetOnDClick(func(_ systray.IMenu) {
		go a.sw.SwitchToPeer()
	})

	a.mWarn = systray.AddMenuItem("显示器没有响应 DDC/CI", "")
	a.mWarn.Disable()
	a.mWarn.Hide()

	a.mSwitch = systray.AddMenuItem("切到对端", "双击托盘图标也可以")
	systray.AddSeparator()

	if a.oinkOK {
		a.mAuto = systray.AddMenuItemCheckbox("自动切屏", "跟着 Oink 的焦点自动切", a.Config().AutoSwitch)
		a.mDelay = systray.AddMenuItem("延迟", "")
		a.delayItem = map[int]*systray.MenuItem{}
		for _, ms := range delayChoices {
			a.delayItem[ms] = a.mDelay.AddSubMenuItem(strconv.Itoa(ms)+" ms", "")
		}
		systray.AddSeparator()
	}

	a.mLocalInfo = systray.AddMenuItem("本机口", "自动读取，不需要设置")
	a.mLocalInfo.Disable()
	a.mPeer = systray.AddMenuItem("对端口", "另一台机器接的口")
	a.peerItems = map[int]*systray.MenuItem{}
	for _, v := range a.candidates() {
		v := v
		item := a.mPeer.AddSubMenuItem(strconv.Itoa(v), "")
		a.peerItems[v] = item
		item.Click(func() { a.setPeer(v) })
	}

	systray.AddSeparator()
	a.mAutoStart = systray.AddMenuItemCheckbox("开机自启", "", isAutostartEnabled())
	mAbout := systray.AddMenuItem("关于 OneScreen", "")
	mQuit := systray.AddMenuItem("退出", "")

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
				a.sw.Resync()
				systray.SetTooltip("OneScreen · " + a.sw.StatusText())
			},
			func(msg string) {
				if msg == "" {
					return
				}
				a.log.Printf("[oink] %s", msg)
			})
	}()

	a.mSwitch.Click(func() { go a.sw.SwitchToPeer() })
	a.mAutoStart.Click(func() { a.toggleAutostart() })
	mAbout.Click(showAbout)
	mQuit.Click(func() {
		a.log.Printf("退出")
		systray.Quit()
	})

	if a.mAuto != nil {
		a.mAuto.Click(func() { a.toggleAuto() })
	}
	if a.mDelay != nil {
		for ms, item := range a.delayItem {
			ms, item := ms, item
			item.Click(func() { a.setDelay(ms, item) })
		}
	}
}

func (a *app) toggleAuto() {
	c := a.Config()
	c.AutoSwitch = a.mAuto.Checked()
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
	a.mDelay.SetTitle(fmt.Sprintf("延迟 %dms", ms))
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
			a.mSwitch.SetTitle(fmt.Sprintf("切到对端 (%d)", cfg.PeerInput))
		} else {
			a.mSwitch.SetTitle("切到对端")
		}
	}
	if a.mLocalInfo != nil {
		name := a.monitor
		if name == "" {
			name = "显示器"
		}
		a.mLocalInfo.SetTitle(fmt.Sprintf("本机口 %d · %s", cfg.LocalInput, name))
	}
	if a.mPeer != nil {
		a.mPeer.SetTitle(fmt.Sprintf("对端口 = %d", cfg.PeerInput))
		for v, item := range a.peerItems {
			if v == cfg.PeerInput {
				item.Check()
			} else {
				item.Uncheck()
			}
		}
	}
	if a.mDelay != nil {
		a.mDelay.SetTitle(fmt.Sprintf("延迟 %dms", cfg.DelayMs))
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

// ---------------------------------------------------------------- 杂项

func showAbout() {
	title := syscall.StringToUTF16Ptr("关于 OneScreen")
	text := syscall.StringToUTF16Ptr(fmt.Sprintf(
		"OneScreen %s\n\n单显示器多设备切换。\n\n配置文件：%s\n日志：%s\n\n%s",
		version, configPath(), filepath.Join(dataDir(), "switch.log"), repoURL))
	const mbOK = 0x0
	const mbIconInfo = 0x40
	procMessageBox.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), mbOK|mbIconInfo)
}

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
		for i := uint32(0); i < 8; i++ {
			var dd displayDevice
			dd.cb = uint32(unsafe.Sizeof(dd))
			r, _, _ := procEnumDisplayDevices.Call(0, uintptr(i), uintptr(unsafe.Pointer(&dd)), 0)
			if r == 0 {
				break
			}
			fmt.Printf("adapter %d: name=%q str=%q flags=0x%x\n", i,
				utf16String(dd.deviceName[:]), utf16String(dd.deviceString[:]), dd.stateFlags)
			var mon displayDevice
			mon.cb = uint32(unsafe.Sizeof(mon))
			r2, _, e2 := procEnumDisplayDevices.Call(uintptr(unsafe.Pointer(&dd.deviceName[0])), 0,
				uintptr(unsafe.Pointer(&mon)), 1)
			fmt.Printf("   monitor r=%d err=%v str=%q id=%q edid=%q\n", r2, e2,
				utf16String(mon.deviceString[:]), utf16String(mon.deviceID[:]),
				edidName(utf16String(mon.deviceID[:])))
		}
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
		fmt.Println("usage: --ddc-list | --ddc-caps [monitor] | --ddc-get [monitor] [code] | --ddc-set <monitor> <value> [code]")
	}
}
