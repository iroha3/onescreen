<div align="center">

<img src="tray.png" width="64" height="64" alt="OneScreen Logo" />

# OneScreen

**单显示器多设备极简切换器**

一块屏幕，多台主机。不用硬件 KVM，不用装第三方 DDC 臃肿全家桶，双击托盘即切。  
配合 [Oink](https://github.com/oinkvm/oink) 可实现鼠标越界全自动切屏。

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20Linux%20%7C%20macOS-informational?style=flat)]()
[![License: AGPL v3](https://img.shields.io/badge/License-AGPLv3-blue.svg)](LICENSE)
[![GitHub release](https://img.shields.io/github/v/release/iroha3/onescreen?include_prereleases&color=orange)](https://github.com/iroha3/onescreen/releases)

</div>

---

## 💡 设计哲学

1. **为单屏多机而生**：很多极客桌面只保留一块高素质单一显示器，通过 DP 与 HDMI 分别接入 PC 与 Mac/工作机。OneScreen 让你彻底告别显示器下沿难按的物理 OSD 按键与昂贵的物理 KVM 盒子。
2. **零多余新手引导**：显示器通常只有 2~3 个有效接口。第一次使用时，对端口默认取候选最小值；切过去黑屏说明选错了，在右键菜单里点选换一个，改一次永久记住，绝不弹窗打扰。
3. **两台机器都装，拷走即用**：配置文件记录了主机名（`machine`）。直接把程序目录拷到第二台机器上运行时，OneScreen 会自动识别并将「本机口」与「对端口」自动对调，省去重复配置。
4. **Oink 绝佳搭档**：配合键鼠穿越工具 [Oink](https://github.com/oinkvm/oink) 使用时，鼠标指针滑到对端屏幕边缘，显示器信源同步瞬切；未运行 Oink 时自动进入静默退避，纯手动切换零日志干扰。
5. **绝对安全逃生出口**：全局应急热键 `Ctrl + Alt + Shift + S`。无论对端发生休眠、锁屏、假死还是信号丢失，随时一键盲操强制切回本机，绝不抓瞎。

---

## 🖥️ 托盘菜单

```text
切到另一台 (HDMI 2)             ← 双击托盘或点此项直接切屏
随 Oink 自动切屏 ▸              ← Oink 连通时可用，断开时变灰提示
   启用 ✓
   延迟 ▸ 200 / 300 / 400 / 600 / 800 / 1000 ms
──────────────────────────────
另一台接的接口 ▸ HDMI 1 (17) / HDMI 2 (18)   已排除当前本机口
──────────────────────────────
开机自启 ✓
帮助/关于…
退出
```

- **双击托盘**：立即切到另一台主机。
- **应急热键**：`Ctrl + Alt + Shift + S`（任意时刻强制切回本机）。
- **多语言**：托盘菜单与系统弹窗自动跟随系统语言（中文 / 英文）。

---

## 🔌 常用显示器输入源速查 (VCP 0x60)

| 端口类型 | VCP 十进制值 | VCP 十六进制值 | 说明 |
| :--- | :--- | :--- | :--- |
| **DisplayPort 1** | `15` | `0x0F` | 主流 PC 显卡主接口 |
| **DisplayPort 2** | `16` | `0x10` | 第二 DP 口 |
| **HDMI 1** | `17` | `0x11` | 常见副机 / 主机 HDMI 接口 |
| **HDMI 2** | `18` | `0x12` | 常见笔记本 / 扩展坞接口 |
| **USB-C / Type-C**| `19` 或 `27` | `0x13` 或 `0x1B` | 全功能 Type-C / 雷电反充接口 |

---

## 🚀 跨平台支持与依赖

OneScreen 针对全平台进行了深度原生适配：

| 平台 | 显示器 DDC 底层驱动 | 对话框与自启实现 | 依赖工具 |
| :--- | :--- | :--- | :--- |
| **Windows** (x64) | 原生 Win32 `dxva2.dll` API | TaskDialogIndirect + 注册表 Run | **零依赖**，单 exe 独立运行 |
| **Linux** (x64/arm64) | `ddcutil` 协议通信 | Zenity / KDialog + XDG Autostart | `sudo apt install ddcutil` |
| **macOS** (Apple Silicon/Intel) | `m1ddc` 或 `ddcctl` | AppleScript + LaunchAgents | `brew install m1ddc` (M系列推荐) |

---

## 🛠️ 编译与构建

### Windows
```powershell
# 注入 Windows PE 清单与高分屏图标（换图标或清单时执行一次）
rsrc -manifest app.manifest -ico icon.ico -o rsrc_windows_amd64.syso -arch amd64

# 编译无控制台 GUI 二进制
go build -ldflags "-H windowsgui -s -w" -o onescreen.exe .
```

### Linux (Ubuntu / Debian / Fedora)
```bash
go build -ldflags "-s -w" -o onescreen .
```

### macOS (Apple Silicon / Intel)
```bash
go build -ldflags "-s -w" -o onescreen .
```

---

## 💻 命令行调试指令

OneScreen 内置了快速命令行测试模式，适合验证显示器 DDC/CI 通信与硬件状态：

```bash
# 列出系统识别到的显示器与支持的信源列表
onescreen --ddc-list

# 查询当前显示器友好名称（通过 EDID / system_profiler 解析）
onescreen --monitor-name

# 实时读取当前显示器信源 (VCP 0x60)
onescreen --ddc-get 0

# 强制将显示器切换到指定输入源（例如切换到 HDMI 2: 18）
onescreen --ddc-set 0 18

# 弹出图形关于与状态窗口
onescreen --about
```

---

## 📄 配置文件说明

- 配置文件 `config.json` 默认优先存放在 **可执行文件同级目录**（绿色便携模式，拷走即用）。
- 当程序位于受写保护的系统目录（如 `C:\Program Files`）时，会自动退回至系统标准用户数据目录（Windows: `%APPDATA%\OneScreen`；Linux: `~/.config/OneScreen`；macOS: `~/Library/Application Support/OneScreen`）。

---

## ⚖️ 开源协议与版权

- **License**: [GNU Affero General Public License v3.0 (AGPLv3)](LICENSE)
- **Copyright**: © 2026 [iroha3](https://github.com/iroha3)
