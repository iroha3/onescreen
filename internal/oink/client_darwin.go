//go:build darwin

package oink

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", "Oink")
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
	candidates := []string{
		filepath.Join(oinkDir, "oink-build.json"),
		filepath.Join(oinkDir, "Contents", "Resources", "oink-build.json"),
		filepath.Join(oinkDir, "Contents", "MacOS", "oink-build.json"),
		filepath.Join(filepath.Dir(oinkDir), "Resources", "oink-build.json"),
	}
	var data []byte
	var err error
	for _, p := range candidates {
		data, err = os.ReadFile(p)
		if err == nil {
			break
		}
	}
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

	var conn net.Conn
	for slot := 0; slot < 32; slot++ {
		sockPath := fmt.Sprintf("%s.%d", ep.Path, slot)
		c, err := net.Dial("unix", sockPath)
		if err == nil {
			conn = c
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
	home, _ := os.UserHomeDir()
	commonDirs := []string{
		"/Applications/Oink.app/Contents/MacOS",
		"/Applications/Oink.app",
		filepath.Join(home, "Applications", "Oink.app", "Contents", "MacOS"),
		filepath.Join(home, "Applications", "Oink.app"),
	}
	for _, dir := range commonDirs {
		if hasBuildFileAnywhere(dir) {
			return dir
		}
	}
	return ""
}

func hasBuildFileAnywhere(dir string) bool {
	if HasBuildFile(dir) {
		return true
	}
	if HasBuildFile(filepath.Join(dir, "Contents", "Resources")) {
		return true
	}
	if HasBuildFile(filepath.Join(dir, "Contents", "MacOS")) {
		return true
	}
	return false
}

func oinkFromRunning() string {
	ep, err := readEndpoint()
	if err != nil || ep.Owner.Pid == 0 {
		return ""
	}
	cmd := exec.Command("ps", "-p", strconv.Itoa(int(ep.Owner.Pid)), "-o", "comm=")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	exePath := strings.TrimSpace(string(out))
	if exePath == "" {
		return ""
	}
	dir := filepath.Dir(exePath)
	if hasBuildFileAnywhere(dir) {
		return dir
	}
	appDir := dir
	for appDir != "/" && appDir != "." {
		if strings.HasSuffix(appDir, ".app") {
			return filepath.Join(appDir, "Contents", "MacOS")
		}
		appDir = filepath.Dir(appDir)
	}
	return dir
}
