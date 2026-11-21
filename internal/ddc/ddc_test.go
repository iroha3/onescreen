package ddc

import (
	"reflect"
	"testing"
)

func TestParseInputsFromCaps(t *testing.T) {
	tests := []struct {
		name string
		caps string
		want []byte
	}{
		{
			name: "Standard capabilities string",
			caps: "prot(monitor)type(LCD)model(F24G30F)cmds(01 02)vcp(02 04 14(05 08) 60(0F 11 12) CC)mccs_ver(2.1)",
			want: []byte{0x0F, 0x11, 0x12},
		},
		{
			name: "Uppercase 60 with multiple spaces",
			caps: "vcp( 60 ( 0F   11   12 ) )",
			want: []byte{0x0F, 0x11, 0x12},
		},
		{
			name: "Decimal/Hex input sources with DP and HDMI",
			caps: "60(11 12 0F 1B)", // 17, 18, 15, 27
			want: []byte{0x11, 0x12, 0x0F, 0x1B},
		},
		{
			name: "Missing 60 feature code",
			caps: "prot(monitor)vcp(04 14 10)",
			want: nil,
		},
		{
			name: "Empty 60 parentheses",
			caps: "vcp(60())",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseInputsFromCaps(tt.caps)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseInputsFromCaps() = %v, want %v", got, tt.want)
			}
		})
	}
}
