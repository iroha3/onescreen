package oink

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"onescreen/internal/platform"
)

// Focus 是我们关心的键鼠焦点事件
type Focus struct {
	IsLocal    bool
	Device     string
	DeviceName string
}

type Subscription struct {
	R     *bufio.Reader
	Close func()
}

func HasBuildFile(dir string) bool {
	if dir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, "oink-build.json"))
	return err == nil
}

// ReadStatus 读一次当前焦点，用于基准对齐
func ReadStatus(agentPath string) (Focus, bool) {
	if agentPath == "" {
		return Focus{}, false
	}
	cmd := exec.Command(agentPath, "status")
	platform.HideWindow(cmd)
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
	if err := json.Unmarshal(out, &doc); err != nil || doc.Overview.Focus == nil {
		return Focus{}, false
	}
	f := doc.Overview.Focus
	return Focus{IsLocal: f.IsLocal, Device: f.Device, DeviceName: f.DeviceName}, true
}

// RunEvents 常驻订阅：连上 → 逐行读 → 断开重连。包含静默与退避逻辑。
func RunEvents(ctx context.Context, oinkDir string, log func(format string, args ...any), onFocus func(Focus), onConnect func(), onProblem func(string)) {
	problem := ""
	report := func(msg string) {
		if msg == problem {
			return
		}
		problem = msg
		if onProblem != nil {
			onProblem(msg)
		}
	}

	loggedOffline := false
	backoff := 5 * time.Second

	for ctx.Err() == nil {
		sub, err := ConnectDirect(oinkDir)
		if err != nil {
			if !loggedOffline {
				if log != nil {
					log("[oink] 未检测到运行中的 Oink（纯手动切换模式）")
				}
				loggedOffline = true
			}
			report("Oink 没在运行")
			sleepCtx(ctx, backoff)
			if backoff < 30*time.Second {
				backoff += 5 * time.Second
			}
			continue
		}

		loggedOffline = false
		backoff = 5 * time.Second
		report("")
		if log != nil {
			log("[oink] 已连接并订阅焦点事件")
		}
		if onConnect != nil {
			onConnect()
		}

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
		report("Oink 没在运行")
		if log != nil {
			log("[oink] 订阅断开，重连中")
		}
		sleepCtx(ctx, 3*time.Second)
	}
}

func readLoop(sub *Subscription, log func(format string, args ...any), onFocus func(Focus)) {
	for {
		line, err := sub.R.ReadString('\n')
		if len(line) > 0 {
			handleLine([]byte(line), log, onFocus)
		}
		if err != nil {
			if err != io.EOF && log != nil {
				log("[oink] 读取异常：%v", err)
			}
			return
		}
	}
}

// handleLine 只找 focus 事件，其余连 JSON 都不解析（稳态下本来也没几条）。
func handleLine(line []byte, log func(format string, args ...any), onFocus func(Focus)) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return
	}
	if bytes.Contains(line, []byte(`"closing"`)) {
		if log != nil {
			log("[oink] 后端关闭订阅：%s", string(line))
		}
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
	if onFocus != nil {
		onFocus(Focus{IsLocal: ev.Data.IsLocal, Device: ev.Data.Device, DeviceName: ev.Data.DeviceName})
	}
}

func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
