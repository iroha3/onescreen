package oink

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHandleLine(t *testing.T) {
	var gotFocus *Focus
	onFocus := func(f Focus) {
		gotFocus = &f
	}

	// 1. Oink 标准包装事件 (is_local: false)
	line1 := []byte(`{"event":{"type":"focus","data":{"device":"dev123","device_name":"DESKTOP-PEER","is_local":false}}}`)
	gotFocus = nil
	handleLine(line1, nil, onFocus)
	if gotFocus == nil {
		t.Fatalf("expected focus event, got nil")
	}
	if gotFocus.IsLocal != false || gotFocus.Device != "dev123" || gotFocus.DeviceName != "DESKTOP-PEER" {
		t.Fatalf("unexpected focus data: %+v", *gotFocus)
	}

	// 2. Oink 标准包装事件 (is_local: true)
	line2 := []byte(`{"event":{"type":"focus","data":{"device":"devLocal","device_name":"DESKTOP-LOCAL","is_local":true}}}`)
	gotFocus = nil
	handleLine(line2, nil, onFocus)
	if gotFocus == nil {
		t.Fatalf("expected focus event, got nil")
	}
	if gotFocus.IsLocal != true || gotFocus.Device != "devLocal" {
		t.Fatalf("unexpected focus data: %+v", *gotFocus)
	}

	// 3. 其它事件（如 log / pause / ui_preferences）应被直接忽略
	line3 := []byte(`{"event":{"type":"log","data":{"message":"something"}}}`)
	gotFocus = nil
	handleLine(line3, nil, onFocus)
	if gotFocus != nil {
		t.Fatalf("expected nil for non-focus event, got %+v", *gotFocus)
	}

	// 4. 无包装直接事件
	line4 := []byte(`{"type":"focus","data":{"device":"direct","device_name":"DIRECT","is_local":false}}`)
	gotFocus = nil
	handleLine(line4, nil, onFocus)
	if gotFocus == nil || gotFocus.IsLocal != false {
		t.Fatalf("expected direct focus event, got %+v", gotFocus)
	}
}

func TestHasBuildFile(t *testing.T) {
	if HasBuildFile("") {
		t.Errorf("empty dir should not have build file")
	}

	tmp := t.TempDir()
	if HasBuildFile(tmp) {
		t.Errorf("empty temp dir should not have build file")
	}

	bf := filepath.Join(tmp, "oink-build.json")
	if err := os.WriteFile(bf, []byte(`{"version":"0.1.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if !HasBuildFile(tmp) {
		t.Errorf("temp dir with oink-build.json should return true")
	}
}
