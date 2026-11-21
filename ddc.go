package main

import (
	"fmt"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

// 内置 DDC/CI：直接调 dxva2.dll，不依赖任何外部 exe。
// 每次操作都重新枚举显示器句柄 —— 切输入源后旧句柄会失效。

const vcpInputSource = 0x60

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	dxva2  = syscall.NewLazyDLL("dxva2.dll")

	procEnumDisplayMonitors    = user32.NewProc("EnumDisplayMonitors")
	procEnumDisplayDevices     = user32.NewProc("EnumDisplayDevicesW")
	procGetNumPhysicalMonitors = dxva2.NewProc("GetNumberOfPhysicalMonitorsFromHMONITOR")
	procGetPhysicalMonitors    = dxva2.NewProc("GetPhysicalMonitorsFromHMONITOR")
	procSetVCPFeature          = dxva2.NewProc("SetVCPFeature")
	procGetVCPFeatureAndReply  = dxva2.NewProc("GetVCPFeatureAndVCPFeatureReply")
	procDestroyPhysicalMonitor = dxva2.NewProc("DestroyPhysicalMonitor")
	procGetCapsLength          = dxva2.NewProc("GetCapabilitiesStringLength")
	procGetCapsReply           = dxva2.NewProc("CapabilitiesRequestAndCapabilitiesReply")
)

// PHYSICAL_MONITOR { HANDLE hPhysicalMonitor; WCHAR szPhysicalMonitorDescription[128]; }
type physicalMonitor struct {
	handle      uintptr
	description [128]uint16
}

type ddcMonitor struct {
	handle uintptr
	desc   string
}

func utf16String(buf []uint16) string {
	var b []rune
	for _, c := range buf {
		if c == 0 {
			break
		}
		b = append(b, rune(c))
	}
	return string(b)
}

func enumMonitors() ([]ddcMonitor, error) {
	var hmons []uintptr
	cb := syscall.NewCallback(func(hMonitor, hdc, rect, data uintptr) uintptr {
		hmons = append(hmons, hMonitor)
		return 1 // 继续枚举
	})
	r, _, err := procEnumDisplayMonitors.Call(0, 0, cb, 0)
	if r == 0 {
		return nil, fmt.Errorf("EnumDisplayMonitors 失败：%v", err)
	}

	var out []ddcMonitor
	for _, hm := range hmons {
		var n uint32
		if r, _, _ := procGetNumPhysicalMonitors.Call(hm, uintptr(unsafe.Pointer(&n))); r == 0 || n == 0 {
			continue
		}
		arr := make([]physicalMonitor, n)
		if r, _, _ := procGetPhysicalMonitors.Call(hm, uintptr(n), uintptr(unsafe.Pointer(&arr[0]))); r == 0 {
			continue
		}
		for _, p := range arr {
			out = append(out, ddcMonitor{handle: p.handle, desc: utf16String(p.description[:])})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("没有可用的物理显示器")
	}
	return out, nil
}

func closeMonitors(ms []ddcMonitor) {
	for _, m := range ms {
		_, _, _ = procDestroyPhysicalMonitor.Call(m.handle)
	}
}

// pickMonitor 选目标：selector 是序号（"0"）或描述里的子串。
func pickMonitor(ms []ddcMonitor, selector string) (ddcMonitor, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" || selector == "0" {
		return ms[0], nil
	}
	if isDigits(selector) {
		var idx int
		fmt.Sscanf(selector, "%d", &idx)
		if idx >= 0 && idx < len(ms) {
			return ms[idx], nil
		}
		return ddcMonitor{}, fmt.Errorf("显示器序号 %d 超出范围（共 %d 个）", idx, len(ms))
	}
	for _, m := range ms {
		if strings.Contains(strings.ToUpper(m.desc), strings.ToUpper(selector)) {
			return m, nil
		}
	}
	return ddcMonitor{}, fmt.Errorf("没找到描述里含 %q 的显示器", selector)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func getVCP(handle uintptr, code byte) (uint32, uint32, error) {
	var codeType, cur, max uint32
	r, _, e := procGetVCPFeatureAndReply.Call(
		handle,
		uintptr(code),
		uintptr(unsafe.Pointer(&codeType)),
		uintptr(unsafe.Pointer(&cur)),
		uintptr(unsafe.Pointer(&max)),
	)
	if r == 0 {
		return 0, 0, fmt.Errorf("读取 VCP 0x%02X 失败：%v", code, e)
	}
	return cur, max, nil
}

func setVCP(handle uintptr, code byte, value uint32) error {
	if r, _, e := procSetVCPFeature.Call(handle, uintptr(code), uintptr(value)); r == 0 {
		return fmt.Errorf("写入 VCP 0x%02X 失败：%v", code, e)
	}
	return nil
}

// ensureInput 是切换的唯一入口：先读当前输入源，已经是目标就不写。
// 这样两台机器同时开自动切也不会互相打架，谁先到谁写，后到的直接跳过。
func ensureInput(selector string, target uint32) (changed bool, err error) {
	ms, err := enumMonitors()
	if err != nil {
		return false, err
	}
	defer closeMonitors(ms)
	m, err := pickMonitor(ms, selector)
	if err != nil {
		return false, err
	}
	if cur, _, err := getVCP(m.handle, vcpInputSource); err == nil && cur == target {
		return false, nil
	}
	if err := setVCP(m.handle, vcpInputSource, target); err != nil {
		return false, err
	}
	return true, nil
}

// ddcGet 读一个 VCP 值（调试用）。
func ddcGet(selector string, code byte) (uint32, uint32, error) {
	ms, err := enumMonitors()
	if err != nil {
		return 0, 0, err
	}
	defer closeMonitors(ms)
	m, err := pickMonitor(ms, selector)
	if err != nil {
		return 0, 0, err
	}
	return getVCP(m.handle, code)
}

// ddcSet 写一个 VCP 值（调试用）。
func ddcSet(selector string, code byte, value uint32) error {
	ms, err := enumMonitors()
	if err != nil {
		return err
	}
	defer closeMonitors(ms)
	m, err := pickMonitor(ms, selector)
	if err != nil {
		return err
	}
	return setVCP(m.handle, code, value)
}

// ddcInputs 读显示器的能力串，取出 VCP 0x60 支持的所有取值。
// 能力串形如：(prot(monitor)type(LCD)vcp(02 04 10 12 60(11 12 0F) AC))，里面是十六进制。
func ddcInputs(selector string) ([]uint32, error) {
	ms, err := enumMonitors()
	if err != nil {
		return nil, err
	}
	defer closeMonitors(ms)
	m, err := pickMonitor(ms, selector)
	if err != nil {
		return nil, err
	}

	var n uint32
	if r, _, e := procGetCapsLength.Call(m.handle, uintptr(unsafe.Pointer(&n))); r == 0 || n == 0 {
		return nil, fmt.Errorf("显示器不返回能力串：%v", e)
	}
	buf := make([]byte, n+1)
	if r, _, e := procGetCapsReply.Call(m.handle, uintptr(unsafe.Pointer(&buf[0])), uintptr(n)); r == 0 {
		return nil, fmt.Errorf("读能力串失败：%v", e)
	}

	caps := string(buf)
	lastCaps = caps
	idx := strings.Index(caps, "60(")
	if idx < 0 {
		return nil, fmt.Errorf("能力串里没有输入源列表")
	}
	rest := caps[idx+3:]
	end := strings.IndexByte(rest, ')')
	if end < 0 {
		return nil, fmt.Errorf("能力串格式异常")
	}
	var out []uint32
	for _, f := range strings.Fields(rest[:end]) {
		if v, err := strconv.ParseUint(f, 16, 32); err == nil {
			out = append(out, uint32(v))
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("能力串里没有可用的输入源")
	}
	return out, nil
}

// lastCaps 留着给 --ddc-caps 看原始能力串。
var lastCaps string

func ddcCaps(selector string) (string, error) {
	if _, err := ddcInputs(selector); err != nil {
		return "", err
	}
	return lastCaps, nil
}

// DISPLAY_DEVICEW
type displayDevice struct {
	cb           uint32
	deviceName   [32]uint16
	deviceString [128]uint16
	stateFlags   uint32
	deviceID     [128]uint16
	deviceKey    [128]uint16
}

const displayDeviceAttachedToDesktop = 0x1

// monitorName 取显示器的友好名（如 F24G30F），拿不到就返回空。
func monitorName() string {
	for i := uint32(0); i < 8; i++ {
		var dd displayDevice
		dd.cb = uint32(unsafe.Sizeof(dd))
		r, _, _ := procEnumDisplayDevices.Call(0, uintptr(i), uintptr(unsafe.Pointer(&dd)), 0)
		if r == 0 {
			break
		}
		if dd.stateFlags&displayDeviceAttachedToDesktop == 0 {
			continue
		}
		var mon displayDevice
		mon.cb = uint32(unsafe.Sizeof(mon))
		r, _, _ = procEnumDisplayDevices.Call(
			uintptr(unsafe.Pointer(&dd.deviceName[0])),
			0,
			uintptr(unsafe.Pointer(&mon)),
			1, // EDD_GET_DEVICE_INTERFACE_NAME
		)
		if r != 0 {
			// deviceString 只有 "Generic PnP Monitor"，真名要从 EDID 里读
			if name := edidName(utf16String(mon.deviceID[:])); name != "" {
				return name
			}
		}
	}
	return ""
}

// edidName 从注册表里的 EDID 解析显示器型号名（0xFC 描述符）。
// deviceID 有两种形态：
//   \\?\DISPLAY#SKY0010#5&2d8d6d81&0&UID16642#{guid}   （带 EDD_GET_DEVICE_INTERFACE_NAME）
//   MONITOR\SKY0010\{4d36e96e-...}\0001
func edidName(deviceID string) string {
	hwid, instance := "", ""
	if strings.HasPrefix(deviceID, `\\?\`) {
		parts := strings.Split(deviceID, "#")
		if len(parts) >= 2 {
			hwid = parts[1]
		}
		if len(parts) >= 3 {
			instance = parts[2]
		}
	} else {
		parts := strings.Split(deviceID, `\`)
		if len(parts) >= 2 {
			hwid = parts[1]
		}
		if len(parts) >= 3 {
			instance = parts[2]
		}
	}
	if hwid == "" {
		return ""
	}
	root, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Enum\DISPLAY\`+hwid, registry.READ)
	if err != nil {
		return ""
	}
	defer root.Close()

	tryOne := func(name string) string {
		sk, err := registry.OpenKey(root, name+`\Device Parameters`, registry.READ)
		if err != nil {
			return ""
		}
		data, _, err := sk.GetBinaryValue("EDID")
		_ = sk.Close()
		if err != nil {
			return ""
		}
		return parseEDIDName(data)
	}
	if instance != "" {
		if n := tryOne(instance); n != "" {
			return n
		}
	}
	insts, _ := root.ReadSubKeyNames(-1)
	for _, inst := range insts {
		if n := tryOne(inst); n != "" {
			return n
		}
	}
	return ""
}

func parseEDIDName(b []byte) string {
	if len(b) < 128 || b[0] != 0x00 || b[1] != 0xFF {
		return ""
	}
	for off := 54; off+18 <= 128; off += 18 {
		d := b[off : off+18]
		if d[0] == 0 && d[1] == 0 && d[3] == 0xFC {
			name := strings.TrimRight(string(d[5:18]), "\x00\n\r ")
			return strings.TrimSpace(name)
		}
	}
	return ""
}
