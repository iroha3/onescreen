package main

import (
	"sync"
	"time"

	"onescreen/internal/ddc"
	"onescreen/internal/oink"
)

type action struct {
	toLocal bool
}

// Switcher 是与 Oink 无关的状态机：延迟、取消、串行。
// 「显示器现在在哪一侧」不再由自己记账，交给 ddc.EnsureInput 直接问显示器。
type Switcher struct {
	mu      sync.Mutex
	cfg     func() Config
	log     *logFile
	onError func(string)

	focus   *oink.Focus
	timer   *time.Timer
	running bool
	pending *action
	paused  bool
	lastTo  *bool
	lastAt  time.Time
}

func NewSwitcher(cfg func() Config, log *logFile, onError func(string)) *Switcher {
	return &Switcher{cfg: cfg, log: log, onError: onError}
}

func (s *Switcher) SetPaused(p bool) {
	s.mu.Lock()
	s.paused = p
	if p && s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.mu.Unlock()
	s.log.Printf("[switch] 自动切屏=%v", !p)
}

func (s *Switcher) Paused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.paused
}

// StatusText 给托盘提示用。
func (s *Switcher) StatusText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.paused {
		return "自动切屏已关闭"
	}
	if s.focus == nil {
		return "等待 Oink…"
	}
	if s.focus.IsLocal {
		return "焦点在本机"
	}
	name := s.focus.DeviceName
	if name == "" {
		name = "另一台"
	}
	return "焦点在 " + name
}

// Resync 用当前焦点重新对齐基准，不触发切换。启动和每次重连都调一次。
func (s *Switcher) Resync() {
	f, ok := oink.ReadStatus(s.cfg().AgentPath)
	if !ok {
		s.log.Printf("[switch] 读不到 Oink 焦点状态")
		return
	}
	s.mu.Lock()
	s.focus = &f
	s.mu.Unlock()
	s.log.Printf("[switch] 基准：焦点%s", localWord(f.IsLocal))
}

func localWord(local bool) string {
	if local {
		return "在本机"
	}
	return "在对端"
}

func (s *Switcher) OnFocus(f oink.Focus) {
	s.mu.Lock()
	if s.focus != nil && s.focus.IsLocal == f.IsLocal && s.focus.Device == f.Device {
		s.mu.Unlock()
		return
	}
	prev := s.focus
	s.focus = &f
	if prev == nil {
		s.mu.Unlock()
		s.log.Printf("[switch] 首个焦点%s", localWord(f.IsLocal))
		return
	}
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	paused := s.paused
	s.mu.Unlock()

	if f.IsLocal == prev.IsLocal {
		return
	}
	s.log.Printf("[focus] %s → %s", localWord(prev.IsLocal), localWord(f.IsLocal))
	if paused {
		return
	}
	s.schedule(action{toLocal: f.IsLocal})
}

func (s *Switcher) schedule(a action) {
	delay := time.Duration(s.cfg().DelayMs) * time.Millisecond
	s.mu.Lock()
	s.timer = time.AfterFunc(delay, func() {
		s.mu.Lock()
		s.timer = nil
		s.mu.Unlock()
		s.fire(a)
	})
	s.mu.Unlock()
}

func (s *Switcher) fire(a action) {
	s.mu.Lock()
	if s.running {
		s.pending = &a
		s.mu.Unlock()
		s.log.Printf("[switch] 上一条还没完成，排队")
		return
	}
	cooldown := time.Duration(s.cfg().CooldownMs) * time.Millisecond
	if s.lastTo != nil && *s.lastTo == a.toLocal && time.Since(s.lastAt) < cooldown {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.lastTo = &a.toLocal
	s.lastAt = time.Now()
	s.mu.Unlock()

	s.apply(a.toLocal)

	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
	s.drain()
}

func (s *Switcher) drain() {
	s.mu.Lock()
	p := s.pending
	s.pending = nil
	s.mu.Unlock()
	if p != nil {
		s.fire(*p)
	}
}

// SwitchToPeer 手动切到对端（双击托盘 / 点菜单）。
func (s *Switcher) SwitchToPeer() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	s.apply(false)

	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
	s.drain()
}

// SwitchToLocal 应急切回本机（逃生热键 / 异常回退）。
func (s *Switcher) SwitchToLocal() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	s.apply(true)

	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
	s.drain()
}

// apply 是唯一真正动显示器的地方。
func (s *Switcher) apply(toLocal bool) {
	cfg := s.cfg()
	target, side := cfg.PeerInput, "对端"
	if toLocal {
		target, side = cfg.LocalInput, "本机"
	}
	if target <= 0 {
		s.log.Printf("[switch] %s输入源还没确定", side)
		return
	}
	changed, err := ddc.EnsureInput(cfg.Monitor, uint32(target))
	if err != nil {
		s.log.Printf("[switch] 切到%s失败：%v", side, err)
		if s.onError != nil {
			s.onError("显示器没有响应 DDC/CI")
		}
		return
	}
	if s.onError != nil {
		s.onError("")
	}
	if changed {
		s.log.Printf("[switch] 已切到%s（输入源 %d）", side, target)
	} else {
		s.log.Printf("[switch] 已经在%s，跳过", side)
	}
}
