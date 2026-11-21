package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

type Config struct {
	// 目标显示器：序号 "0" 或描述子串
	Monitor string `json:"monitor"`
	// 本机接的输入源，自动读取，不对外开放修改
	LocalInput int `json:"local_input"`
	// 另一台机器接的输入源，用户从候选里选一次
	PeerInput int `json:"peer_input"`
	// 焦点来源（决定菜单文案和后端实现），目前只有 "oink"
	FocusSource string `json:"focus_source"`
	// 跟随焦点自动切屏
	AutoSwitch bool `json:"auto_switch"`
	DelayMs    int  `json:"delay_ms"`
	CooldownMs int  `json:"cooldown_ms"`
	// Oink 安装信息，自动探测
	OinkDir   string `json:"oink_dir"`
	AgentPath string `json:"agent_path"`
	Autostart bool   `json:"autostart"`
	// 显示器支持的输入源缓存（读能力串要 1.8 秒，不能放在启动路径上）
	InputsCache []int `json:"inputs_cache,omitempty"`
	// 这份配置属于哪台机器：拷到另一台时靠它发现并对调两个口
	Machine string `json:"machine,omitempty"`
}

var (
	dirOnce sync.Once
	dirVal  string
)

// dataDir 优先用 exe 同级目录（绿色软件，拷走就能用）；
// 放在 Program Files 这类不可写的位置时退回 %APPDATA%\OneScreen。
func dataDir() string {
	dirOnce.Do(func() {
		if exe, err := os.Executable(); err == nil {
			d := filepath.Dir(exe)
			probe := filepath.Join(d, ".write-test")
			if f, err := os.Create(probe); err == nil {
				_ = f.Close()
				_ = os.Remove(probe)
				dirVal = d
				return
			}
		}
		base := os.Getenv("APPDATA")
		if base == "" {
			home, _ := os.UserHomeDir()
			if runtime.GOOS == "darwin" {
				base = filepath.Join(home, "Library", "Application Support")
			} else if runtime.GOOS == "linux" {
				base = os.Getenv("XDG_CONFIG_HOME")
				if base == "" {
					base = filepath.Join(home, ".config")
				}
			} else {
				base = filepath.Join(home, "AppData", "Roaming")
			}
		}
		dirVal = filepath.Join(base, "OneScreen")
		_ = os.MkdirAll(dirVal, 0o755)
	})
	return dirVal
}

func configPath() string { return filepath.Join(dataDir(), "config.json") }

func defaultConfig() Config {
	return Config{
		Monitor:     "0",
		DelayMs:     400,
		CooldownMs:  1000,
		FocusSource: "oink",
		AutoSwitch:  true,
	}
}

func loadConfig(log *logFile) Config {
	cfg := defaultConfig()
	migrateOldConfig(log)
	if data, err := os.ReadFile(configPath()); err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			log.Printf("配置解析失败（%v），改用默认值", cfg)
			cfg = defaultConfig()
		}
		// 同一目录下的旧版配置：字段名不一样，单独认一次
		if cfg.LocalInput == 0 {
			if prev, ok := parseLegacy(data); ok {
				cfg.LocalInput = prev.local
				cfg.PeerInput = prev.peer
				if len(prev.cache) > 0 {
					cfg.InputsCache = prev.cache
				}
				if prev.delay > 0 {
					cfg.DelayMs = prev.delay
				}
				log.Printf("[config] 已从旧字段迁移")
			}
		}
	}
	if cfg.Monitor == "" {
		cfg.Monitor = "0"
	}
	if cfg.FocusSource == "" {
		cfg.FocusSource = "oink"
	}
	if cfg.DelayMs <= 0 {
		cfg.DelayMs = 400
	}
	if cfg.CooldownMs < 0 {
		cfg.CooldownMs = 0
	}
	// 把补全后的默认值写回去，配置文件里始终能看到所有字段
	saveConfig(cfg, log)
	return cfg
}

type legacyConfig struct {
	local, peer int
	cache       []int
	delay       int
}

// parseLegacy 认上一版的字段名（ddc_input_local / ddc_input_remote）。
func parseLegacy(data []byte) (legacyConfig, bool) {
	var prev struct {
		DelayMs     int   `json:"delay_ms"`
		Local       int   `json:"ddc_input_local"`
		Remote      int   `json:"ddc_input_remote"`
		InputsCache []int `json:"ddc_inputs_cache"`
	}
	if json.Unmarshal(data, &prev) != nil || (prev.Local == 0 && prev.Remote == 0) {
		return legacyConfig{}, false
	}
	return legacyConfig{local: prev.Local, peer: prev.Remote, cache: prev.InputsCache, delay: prev.DelayMs}, true
}

// migrateOldConfig 把上一版（OinkSwitch）的配置搬过来，只搬一次。
func migrateOldConfig(log *logFile) {
	if _, err := os.Stat(configPath()); err == nil {
		return
	}
	base := os.Getenv("APPDATA")
	if base == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(base, "OinkSwitch", "config.json"))
	if err != nil {
		return
	}
	// 旧结构：ddc_input_local / ddc_input_remote / ddc_inputs_cache
	var prev struct {
		DelayMs     int   `json:"delay_ms"`
		Local       int   `json:"ddc_input_local"`
		Remote      int   `json:"ddc_input_remote"`
		InputsCache []int `json:"ddc_inputs_cache"`
		Autostart   bool  `json:"autostart"`
	}
	cfg := defaultConfig()
	if json.Unmarshal(data, &prev) == nil {
		cfg.LocalInput = prev.Local
		cfg.PeerInput = prev.Remote
		cfg.InputsCache = prev.InputsCache
		cfg.Autostart = prev.Autostart
		if prev.DelayMs > 0 {
			cfg.DelayMs = prev.DelayMs
		}
	}
	saveConfig(cfg, log)
	log.Printf("[config] 已从旧版本迁移配置")
}

// savedAt 记住自己刚写过的文件时间，避免监听器把自己写的当成外部修改。
var (
	savedMu sync.Mutex
	savedAt time.Time
)

func saveConfig(cfg Config, log *logFile) {
	data, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(configPath(), data, 0o644); err != nil {
		log.Printf("保存配置失败：%v", err)
		return
	}
	if st, err := os.Stat(configPath()); err == nil {
		savedMu.Lock()
		savedAt = st.ModTime()
		savedMu.Unlock()
	}
}

// selfWritten 判断这个修改时间是不是我们自己刚写的。
func selfWritten(t time.Time) bool {
	savedMu.Lock()
	defer savedMu.Unlock()
	return !t.After(savedAt)
}
