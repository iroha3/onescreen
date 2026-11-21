//go:build linux

package ddc

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func EnumMonitors() ([]Monitor, error) {
	out, err := exec.Command("ddcutil", "detect", "--brief").Output()
	if err != nil {
		return nil, fmt.Errorf("ddcutil detect 失败：%v", err)
	}
	lines := strings.Split(string(out), "\n")
	var ms []Monitor
	for i, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "Display") {
			ms = append(ms, Monitor{Handle: uintptr(i), Desc: l})
		}
	}
	if len(ms) == 0 {
		ms = append(ms, Monitor{Handle: 0, Desc: "Default Monitor"})
	}
	return ms, nil
}

func CloseMonitors(ms []Monitor) {}

func MonitorName() string {
	out, err := exec.Command("ddcutil", "detect").Output()
	if err != nil {
		return ""
	}
	re := regexp.MustCompile(`(?i)Model:\s*(.+)`)
	m := re.FindStringSubmatch(string(out))
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func Caps(selector string) (string, error) {
	out, err := exec.Command("ddcutil", "capabilities").Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func Inputs(selector string) ([]uint32, error) {
	caps, err := Caps(selector)
	if err != nil {
		return nil, err
	}
	bs := ParseInputsFromCaps(caps)
	var out []uint32
	for _, b := range bs {
		out = append(out, uint32(b))
	}
	return out, nil
}

func GetVCP(selector string, code byte) (uint32, uint32, error) {
	vcpHex := fmt.Sprintf("%02X", code)
	out, err := exec.Command("ddcutil", "getvcp", vcpHex, "--brief").Output()
	if err != nil {
		return 0, 0, fmt.Errorf("读取 VCP 0x%02X 失败：%v", code, err)
	}
	fields := strings.Fields(string(out))
	if len(fields) >= 4 {
		cur, err1 := strconv.ParseUint(fields[3], 0, 32)
		var max uint64 = 255
		if len(fields) >= 5 {
			max, _ = strconv.ParseUint(fields[4], 0, 32)
		}
		if err1 == nil {
			return uint32(cur), uint32(max), nil
		}
	}
	return 0, 0, fmt.Errorf("解析 ddcutil 输出失败: %s", string(out))
}

func SetVCP(selector string, code byte, value uint32) error {
	vcpHex := fmt.Sprintf("%02X", code)
	valHex := fmt.Sprintf("0x%02X", value)
	out, err := exec.Command("ddcutil", "setvcp", vcpHex, valHex).CombinedOutput()
	if err != nil {
		return fmt.Errorf("写入 VCP 0x%02X 失败: %v (%s)", code, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func EnsureInput(selector string, target uint32) (changed bool, err error) {
	cur, _, err := GetVCP(selector, VCPInputSource)
	if err == nil && cur == target {
		return false, nil
	}
	if err := SetVCP(selector, VCPInputSource, target); err != nil {
		return false, err
	}
	return true, nil
}
