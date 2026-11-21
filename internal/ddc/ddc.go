package ddc

import (
	"regexp"
	"strconv"
	"strings"
)

const VCPInputSource = 0x60

type Monitor struct {
	Handle uintptr
	Desc   string
}

// ParseInputsFromCaps 从能力串（capabilities）中提取 0x60 输入源列表
func ParseInputsFromCaps(caps string) []byte {
	re := regexp.MustCompile(`(?i)60\s*\(([^)]+)\)`)
	m := re.FindStringSubmatch(caps)
	if len(m) < 2 {
		return nil
	}
	fields := strings.Fields(m[1])
	var out []byte
	for _, f := range fields {
		v, err := strconv.ParseUint(f, 16, 8)
		if err == nil {
			out = append(out, byte(v))
		}
	}
	return out
}
