package main

import (
	"reflect"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()
	if cfg.DelayMs != 400 {
		t.Errorf("expected default delay 400ms, got %d", cfg.DelayMs)
	}
	if cfg.CooldownMs != 1000 {
		t.Errorf("expected default cooldown 1000ms, got %d", cfg.CooldownMs)
	}
	if cfg.FocusSource != "oink" {
		t.Errorf("expected default focus source 'oink', got %q", cfg.FocusSource)
	}
	if !cfg.AutoSwitch {
		t.Errorf("expected default auto_switch true, got %v", cfg.AutoSwitch)
	}
}

func TestPortNames(t *testing.T) {
	tests := []struct {
		val       int
		wantShort string
	}{
		{15, "DP 1"},
		{16, "DP 2"},
		{17, "HDMI 1"},
		{18, "HDMI 2"},
		{19, "USB-C"},
		{27, "USB-C"},
		{99, "99"},
	}

	for _, tt := range tests {
		got := portShortName(tt.val)
		if got != tt.wantShort {
			t.Errorf("portShortName(%d) = %q, want %q", tt.val, got, tt.wantShort)
		}
	}
}

func TestCandidates(t *testing.T) {
	// 1. 无缓存时取默认 15, 17, 18 排除本机口
	a := &app{
		cfg: Config{LocalInput: 15},
	}
	cands := a.candidates()
	want := []int{17, 18}
	if !reflect.DeepEqual(cands, want) {
		t.Errorf("candidates() = %v, want %v", cands, want)
	}

	// 2. 有缓存时排除本机口并升序排序
	a2 := &app{
		cfg: Config{
			LocalInput:  18,
			InputsCache: []int{27, 15, 18, 17},
		},
	}
	cands2 := a2.candidates()
	want2 := []int{15, 17, 27}
	if !reflect.DeepEqual(cands2, want2) {
		t.Errorf("candidates() = %v, want %v", cands2, want2)
	}
}

func TestMachineSwapLogic(t *testing.T) {
	cfg := Config{
		Machine:    "HOST-A",
		LocalInput: 15,
		PeerInput:  18,
	}

	// 模拟在不同机器 HOST-B 上加载
	currentHost := "HOST-B"
	if cfg.Machine != "" && cfg.Machine != currentHost {
		cfg.LocalInput, cfg.PeerInput = cfg.PeerInput, cfg.LocalInput
		cfg.Machine = currentHost
	}

	if cfg.LocalInput != 18 || cfg.PeerInput != 15 {
		t.Errorf("expected swapped ports (local:18, peer:15), got (local:%d, peer:%d)",
			cfg.LocalInput, cfg.PeerInput)
	}
	if cfg.Machine != "HOST-B" {
		t.Errorf("expected machine updated to HOST-B, got %s", cfg.Machine)
	}
}
