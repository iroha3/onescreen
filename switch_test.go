package main

import (
	"strings"
	"testing"

	"onescreen/internal/oink"
)

func TestSwitcherStatusText(t *testing.T) {
	sw := NewSwitcher(func() Config {
		return Config{DelayMs: 200, CooldownMs: 500}
	}, nil, nil)

	// 1. 默认无焦点状态
	if sw.StatusText() != "等待 Oink…" {
		t.Errorf("expected '等待 Oink…', got %q", sw.StatusText())
	}

	// 2. 暂停状态
	sw.SetPaused(true)
	if sw.StatusText() != "自动切屏已关闭" {
		t.Errorf("expected '自动切屏已关闭', got %q", sw.StatusText())
	}
	sw.SetPaused(false)

	// 3. 本机焦点
	sw.OnFocus(oink.Focus{IsLocal: true, Device: "d1", DeviceName: "MY-PC"})
	if sw.StatusText() != "焦点在本机" {
		t.Errorf("expected '焦点在本机', got %q", sw.StatusText())
	}

	// 4. 对端焦点
	sw.OnFocus(oink.Focus{IsLocal: false, Device: "d2", DeviceName: "MACBOOK"})
	if sw.StatusText() != "焦点在 MACBOOK" {
		t.Errorf("expected '焦点在 MACBOOK', got %q", sw.StatusText())
	}
}

func TestLocalWord(t *testing.T) {
	if !strings.Contains(localWord(true), "本机") {
		t.Errorf("expected 本机 for true, got %s", localWord(true))
	}
	if !strings.Contains(localWord(false), "对端") {
		t.Errorf("expected 对端 for false, got %s", localWord(false))
	}
}
