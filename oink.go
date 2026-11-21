package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Focus 是我们唯一关心的事件内容。
type Focus struct {
	IsLocal    bool
	Device     string
	DeviceName string
}

type endpoint struct {
	Path  string `json:"path"`
	Token string `json:"token"`
	Owner struct {
		Pid uint32 `json:"pid"`
	} `json:"owner"`
}

type buildID struct {
	Version string `json:"version"`
	Build   string `json:"build"`
	Source  string `json:"source"`
}

type oinkBuildFile struct {
	Version          string `json:"version"`
	Build            int    `json:"build"`
	SourceTreeSHA256 string `json:"source_tree_sha256"`
}

// liveViews 全 false：只要状态变化，不要测量类事件（它们占 557KB/s 的 99%）。
type liveViews struct {
	Stats        bool `json:"stats"`
	CardSignals  bool `json:"card_signals"`
	GroupQuality bool `json:"group_quality"`
	AudioQuality bool `json:"audio_quality"`
	Diagnostics  bool `json:"diagnostics"`
}

// envelope 的字段必须与 oink 完全一致：后端开了 deny_unknown_fields。
type envelope struct {
	Token     string     `json:"token"`
	Build     *buildID   `json:"build,omitempty"`
	Subscribe bool       `json:"subscribe,omitempty"`
	LiveViews *liveViews `json:"live_views,omitempty"`
}

type subscription struct {
	r     *bufio.Reader
	close func()
}

func (s *subscription) Close() {
	if s.close != nil {
		s.close()
	}
}

// ---------------------------------------------------------------- 找 Oink

func hasBuildFile(dir string) bool {
	if dir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, "oink-build.json"))
	return err == nil
}

// findOinkInstall 按可靠性从高到低找安装目录：
// 正在运行的后台进程的镜像路径 → 卸载项的 InstallLocation → 自启项里的 exe 路径 → 常见路径。
// 第一项最准：它是此刻真正在跑的那个 exe，不受"装了几份""从哪启动"影响。
func findOinkInstall() string {
	if dir := oinkFromRunning(); dir != "" {
		return dir
	}
	if dir := oinkFromUninstall(); hasBuildFile(dir) {
		return dir
	}
	if dir := oinkFromRunKey(); hasBuildFile(dir) {
		return dir
	}
	for _, dir := range []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Oink"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Oink"),
		`C:\Program Files\Oink`,
	} {
		if hasBuildFile(dir) {
			return dir
		}
	}
	return ""
}

// oinkFromRunning 从 ipc.json 里的 pid 反查进程镜像路径。
func oinkFromRunning() string {
	ep, err := readEndpoint()
	if err != nil || ep.Owner.Pid == 0 {
		return ""
	}
	img := processImagePath(ep.Owner.Pid)
	if img == "" {
		return ""
	}
	dir := filepath.Dir(img)
	if hasBuildFile(dir) {
		return dir
	}
	return ""
}

func processImagePath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}

func oinkFromUninstall() string {
	type root struct {
		key  registry.Key
		path string
	}
	for _, r := range []root{
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
	} {
		k, err := registry.OpenKey(r.key, r.path, registry.READ)
		if err != nil {
			continue
		}
		names, _ := k.ReadSubKeyNames(-1)
		for _, name := range names {
			sk, err := registry.OpenKey(k, name, registry.READ)
			if err != nil {
				continue
			}
			display, _, _ := sk.GetStringValue("DisplayName")
			if !strings.Contains(strings.ToLower(display), "oink") {
				_ = sk.Close()
				continue
			}
			loc, _, err := sk.GetStringValue("InstallLocation")
			_ = sk.Close()
			if err == nil && loc != "" && hasBuildFile(loc) {
				_ = k.Close()
				return strings.TrimRight(loc, `\`)
			}
		}
		_ = k.Close()
	}
	return ""
}

func oinkFromRunKey() string {
	for _, key := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		k, err := registry.OpenKey(key, `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, registry.READ)
		if err != nil {
			continue
		}
		val, _, err := k.GetStringValue("Oink")
		_ = k.Close()
		if err != nil || val == "" {
			continue
		}
		// 形如 "C:\Program Files\Oink\oink.exe" --autostart
		exe := val
		if strings.HasPrefix(exe, `"`) {
			if end := strings.Index(exe[1:], `"`); end >= 0 {
				exe = exe[1 : end+1]
			}
		} else if i := strings.IndexAny(exe, " \t"); i > 0 {
			exe = exe[:i]
		}
		return filepath.Dir(exe)
	}
	return ""
}

func oinkDataDir() string {
	if d := os.Getenv("LOCALAPPDATA"); d != "" {
		return filepath.Join(d, "Oink")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "AppData", "Local", "Oink")
}

// ---------------------------------------------------------------- 连 Oink

func readEndpoint() (endpoint, error) {
	var e endpoint
	data, err := os.ReadFile(filepath.Join(oinkDataDir(), "ipc.json"))
	if err != nil {
		return e, fmt.Errorf("Oink 没有在运行")
	}
	if err := json.Unmarshal(data, &e); err != nil {
		return e, fmt.Errorf("ipc.json 解析失败")
	}
	return e, nil
}

func readBuild(oinkDir string) (buildID, error) {
	var b buildID
	data, err := os.ReadFile(filepath.Join(oinkDir, "oink-build.json"))
	if err != nil {
		return b, err
	}
	var f oinkBuildFile
	if err := json.Unmarshal(data, &f); err != nil {
		return b, err
	}
	return buildID{Version: f.Version, Build: fmt.Sprint(f.Build), Source: f.SourceTreeSHA256}, nil
}

// connectDirect 打开命名管道并订阅。基名后面要逐个试 32 个槽位。
func connectDirect(cfg Config) (*subscription, error) {
	ep, err := readEndpoint()
	if err != nil {
		return nil, err
	}
	build, err := readBuild(cfg.OinkDir)
	if err != nil {
		return nil, fmt.Errorf("读不到 oink-build.json")
	}

	var conn *os.File
	for slot := 0; slot < 32; slot++ {
		f, err := os.OpenFile(fmt.Sprintf("%s.%d", ep.Path, slot), os.O_RDWR, 0)
		if err == nil {
			conn = f
			break
		}
	}
	if conn == nil {
		return nil, fmt.Errorf("连不上 Oink 的本地接口")
	}

	env := envelope{
		Token:     ep.Token,
		Build:     &build,
		Subscribe: true,
		LiveViews: &liveViews{},
	}
	payload, _ := json.Marshal(env)
	if _, err := conn.Write(append(payload, '\n')); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("订阅失败")
	}

	br := bufio.NewReaderSize(conn, 1<<16)
	reply, err := br.ReadString('\n')
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("Oink 没有应答")
	}
	if strings.Contains(reply, `"closing"`) {
		_ = conn.Close()
		return nil, fmt.Errorf("Oink 拒绝了订阅（版本不匹配）")
	}
	return &subscription{r: br, close: func() { _ = conn.Close() }}, nil
}

// runEvents 常驻订阅：连上 → 逐行读 → 断开重连。每次连上都回调一次做基准对齐。
func runEvents(ctx context.Context, cfg func() Config, log *logFile, onFocus func(Focus), onConnect func(), onProblem func(string)) {
	problem := ""
	for ctx.Err() == nil {
		sub, err := connectDirect(cfg())
		if err != nil {
			if msg := err.Error(); msg != problem {
				problem = msg
				log.Printf("[oink] %s，稍后重试", msg)
				onProblem(msg)
			}
			sleepCtx(ctx, 5*time.Second)
			continue
		}
		if problem != "" {
			problem = ""
			onProblem("")
		}
		log.Printf("[oink] 已订阅焦点事件")
		onConnect()

		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				sub.Close()
			case <-done:
			}
		}()

		readLoop(sub, log, onFocus)
		close(done)
		sub.Close()

		if ctx.Err() != nil {
			return
		}
		log.Printf("[oink] 订阅断开，重连中")
		sleepCtx(ctx, 3*time.Second)
	}
}

func readLoop(sub *subscription, log *logFile, onFocus func(Focus)) {
	for {
		line, err := sub.r.ReadString('\n')
		if len(line) > 0 {
			handleLine([]byte(line), log, onFocus)
		}
		if err != nil {
			if err != io.EOF {
				log.Printf("[oink] 读取失败：%v", err)
			}
			return
		}
	}
}

// handleLine 只找 focus 事件，其余连 JSON 都不解析（稳态下本来也没几条）。
func handleLine(line []byte, log *logFile, onFocus func(Focus)) {
	if bytes.Contains(line, []byte(`"closing"`)) {
		log.Printf("[oink] 后端关闭订阅：%s", strings.TrimSpace(string(line)))
		return
	}
	if !bytes.Contains(line, []byte(`"type":"focus"`)) {
		return
	}
	var wrapper struct {
		Event json.RawMessage `json:"event"`
	}
	body := line
	if json.Unmarshal(line, &wrapper) == nil && len(wrapper.Event) > 0 {
		body = wrapper.Event
	}
	var ev struct {
		Type string `json:"type"`
		Data *struct {
			Device     string `json:"device"`
			DeviceName string `json:"device_name"`
			IsLocal    bool   `json:"is_local"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &ev); err != nil || ev.Type != "focus" || ev.Data == nil {
		return
	}
	onFocus(Focus{IsLocal: ev.Data.IsLocal, Device: ev.Data.Device, DeviceName: ev.Data.DeviceName})
}

// readStatus 用公开 CLI 读一次当前焦点，用于启动和重连后的基准对齐。
func readStatus(cfg Config) (Focus, bool) {
	if cfg.AgentPath == "" {
		return Focus{}, false
	}
	cmd := exec.Command(cfg.AgentPath, "status")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return Focus{}, false
	}
	var doc struct {
		Overview struct {
			Focus *struct {
				Device     string `json:"device"`
				DeviceName string `json:"device_name"`
				IsLocal    bool   `json:"is_local"`
			} `json:"focus"`
		} `json:"overview"`
	}
	if json.Unmarshal(out, &doc) != nil || doc.Overview.Focus == nil {
		return Focus{}, false
	}
	f := doc.Overview.Focus
	return Focus{IsLocal: f.IsLocal, Device: f.Device, DeviceName: f.DeviceName}, true
}

func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
