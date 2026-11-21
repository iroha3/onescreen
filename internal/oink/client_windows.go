//go:build windows

package oink

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

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

type liveViews struct {
	Stats        bool `json:"stats"`
	CardSignals  bool `json:"card_signals"`
	GroupQuality bool `json:"group_quality"`
	AudioQuality bool `json:"audio_quality"`
	Diagnostics  bool `json:"diagnostics"`
}

type envelope struct {
	Token     string     `json:"token"`
	Build     *buildID   `json:"build,omitempty"`
	Subscribe bool       `json:"subscribe,omitempty"`
	LiveViews *liveViews `json:"live_views,omitempty"`
}

func oinkDataDir() string {
	if d := os.Getenv("LOCALAPPDATA"); d != "" {
		return filepath.Join(d, "Oink")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "AppData", "Local", "Oink")
}

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

func ConnectDirect(oinkDir string) (*Subscription, error) {
	ep, err := readEndpoint()
	if err != nil {
		return nil, err
	}
	build, err := readBuild(oinkDir)
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
	return &Subscription{R: br, Close: func() { _ = conn.Close() }}, nil
}

func FindInstall() string {
	if dir := oinkFromRunning(); dir != "" {
		return dir
	}
	if dir := oinkFromUninstall(); HasBuildFile(dir) {
		return dir
	}
	if dir := oinkFromRunKey(); HasBuildFile(dir) {
		return dir
	}
	for _, dir := range []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Oink"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Oink"),
		`C:\Program Files\Oink`,
	} {
		if HasBuildFile(dir) {
			return dir
		}
	}
	return ""
}

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
	if HasBuildFile(dir) {
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
			if err == nil && loc != "" && HasBuildFile(loc) {
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
