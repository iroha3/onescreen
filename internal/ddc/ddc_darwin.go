//go:build darwin

package ddc

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

var lastCaps string

func LastCaps() string {
	return lastCaps
}

func findDdcTool() (string, string) {
	// 优先寻找 m1ddc (Apple Silicon 首选工具)
	for _, p := range []string{"m1ddc", "/opt/homebrew/bin/m1ddc", "/usr/local/bin/m1ddc"} {
		if path, err := exec.LookPath(p); err == nil {
			return "m1ddc", path
		}
	}
	// 其次寻找 ddcctl (Intel Mac 经典工具)
	for _, p := range []string{"ddcctl", "/opt/homebrew/bin/ddcctl", "/usr/local/bin/ddcctl"} {
		if path, err := exec.LookPath(p); err == nil {
			return "ddcctl", path
		}
	}
	return "", ""
}

func EnumMonitors() ([]Monitor, error) {
	tool, toolBin := findDdcTool()
	if tool == "" {
		return nil, fmt.Errorf("macOS 上未检测到 DDC 控制工具，请通过 brew 安装 m1ddc (推荐) 或 ddcctl")
	}

	if tool == "m1ddc" {
		out, err := exec.Command(toolBin, "display", "list").Output()
		if err == nil {
			lines := strings.Split(string(out), "\n")
			var ms []Monitor
			for i, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				ms = append(ms, Monitor{
					Handle: uintptr(i + 1),
					Desc:   line,
				})
			}
			if len(ms) > 0 {
				return ms, nil
			}
		}
	}

	// 默认兜底主显示器
	return []Monitor{
		{Handle: 1, Desc: MonitorName()},
	}, nil
}

func CloseMonitors(ms []Monitor) {}

func MonitorName() string {
	// 通过 system_profiler SPDisplaysDataType 读取显示器名称
	out, err := exec.Command("system_profiler", "SPDisplaysDataType").Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasSuffix(line, ":") && !strings.Contains(line, "Graphics") && !strings.Contains(line, "Chipset") && !strings.Contains(line, "Displays") {
				name := strings.TrimSuffix(line, ":")
				if !strings.Contains(strings.ToLower(name), "built-in") && !strings.Contains(name, "Color LCD") {
					return name
				}
			}
		}
	}
	return "External Display"
}

func GetVCP(selector string, code byte) (uint32, uint32, error) {
	tool, toolBin := findDdcTool()
	if tool == "" {
		return 0, 0, fmt.Errorf("未安装 m1ddc 或 ddcctl")
	}

	if tool == "m1ddc" {
		cmd := exec.Command(toolBin, "get", "input")
		out, err := cmd.Output()
		if err != nil {
			return 0, 0, err
		}
		s := strings.TrimSpace(string(out))
		v, err := strconv.ParseUint(s, 10, 32)
		if err != nil {
			for _, field := range strings.Fields(s) {
				if n, e := strconv.ParseUint(field, 10, 32); e == nil {
					return uint32(n), 255, nil
				}
			}
			return 0, 0, fmt.Errorf("无法解析输入源：%s", s)
		}
		return uint32(v), 255, nil
	}

	// ddcctl: ddcctl -d 1 -i ?
	cmd := exec.Command(toolBin, "-d", "1", "-i", "?")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, 0, err
	}
	text := string(out)
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "input") || strings.Contains(line, "current:") {
			for _, field := range strings.Fields(line) {
				if n, e := strconv.ParseUint(field, 10, 32); e == nil {
					return uint32(n), 255, nil
				}
			}
		}
	}
	return 0, 0, fmt.Errorf("无法读取当前输入源")
}

func SetVCP(selector string, code byte, value uint32) error {
	tool, toolBin := findDdcTool()
	if tool == "" {
		return fmt.Errorf("未安装 m1ddc 或 ddcctl")
	}

	if tool == "m1ddc" {
		cmd := exec.Command(toolBin, "set", "input", strconv.Itoa(int(value)))
		return cmd.Run()
	}

	cmd := exec.Command(toolBin, "-d", "1", "-i", strconv.Itoa(int(value)))
	return cmd.Run()
}

func EnsureInput(selector string, target uint32) (changed bool, err error) {
	if cur, _, err := GetVCP(selector, VCPInputSource); err == nil && cur == target {
		return false, nil
	}
	if err := SetVCP(selector, VCPInputSource, target); err != nil {
		return false, err
	}
	return true, nil
}

func Caps(selector string) (string, error) {
	return "", fmt.Errorf("macOS 暂不支持直接读取能力串")
}

func Inputs(selector string) ([]uint32, error) {
	return []uint32{15, 16, 17, 18, 27}, nil
}
