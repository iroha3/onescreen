<div align="center">

<img src="tray.png" width="80" height="80" alt="OneScreen Icon" />

# OneScreen

**单显示器多机极简切屏工具 · 告别物理 KVM 盒子与 OSD 物理按键**

一块高素质显示器，两台主机。双击托盘即切；配合 [Oink](https://github.com/oinkvm/oink) 更可实现鼠标划过屏幕边缘瞬间自动切屏。

[![GitHub release](https://img.shields.io/github/v/release/iroha3/onescreen?color=orange&label=Release)](https://github.com/iroha3/onescreen/releases/latest)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20macOS%20%7C%20Linux-blue?logo=apple&logoColor=white)]()
[![License: AGPL v3](https://img.shields.io/badge/License-AGPLv3-green.svg)](LICENSE)
[![Zero Hardware](https://img.shields.io/badge/Hardware-Zero%20KVM%20Needed-purple)]()

[📥 下载最新版本](https://github.com/iroha3/onescreen/releases/latest) · [🌐 访问产品主页](https://iroha3.github.io/onescreen/) · [💡 常见疑问解答](#-常见问题-faq)

</div>

---

## ⚡ 为什么选择 OneScreen？

| 方案 | 成本 | 桌面整洁度 | 切换体验 | 高刷/画质损耗 |
| :--- | :--- | :--- | :--- | :--- |
| **物理 KVM 切换器** | 昂贵 (¥300~¥2000+) | 极其杂乱（线缆成倍增加） | 需伸手按物理按键 | 易损失 4K 144Hz / HDR / G-Sync |
| **显示器实体 OSD 按键** | 免费 | 保持原样 | 繁琐难按（需按3~4次二级菜单） | 无损耗但按键易老化断裂 |
| **OneScreen** | **开源免费** | **零新增线缆，极致纯粹** | **鼠标滑过边缘全自动切 / 双击秒切** | **直连显示器，100% 满血原生画质** |

---

## ✨ 核心特性

- 🖱️ **鼠标穿越，信源同行**：作为 [Oink](https://github.com/oinkvm/oink) 的最佳拍档，鼠标光标滑到对侧屏幕边缘，显示器信源同步切过去，体验丝滑如单一大桌面。
- ⚡ **单机掌控全局**：只需在其中一台主机上运行，即可精确捕获光标进出事件，单机全权调度显示器两端信源。
- 🛡️ **双重防抖与硬件保护**：内置 **400ms 划界缓冲** 与 **1000ms 冷却机制**，误碰边缘直接忽略，杜绝频繁振荡，保护显示器主控芯片。
- 🛟 **一键逃生热键**：全局快捷键 `Ctrl + Alt + Shift + S`。无论对端发生蓝屏、休眠还是无信号，随时盲操瞬切回本机。
- 🔄 **拷走即用，双机对调**：若在两台机器都部署，配置文件拷走即用，OneScreen 会根据主机名自动将「本机口」与「对端口」对调，免除二次配置。
- 🪶 **单文件绿色免安装**：内存占用不足 10MB，托盘刻意保留低侵扰极简质感，多语言无缝适配（简体中文 / 英文）。

---

## 🚀 30 秒上手指南

### 1. 下载即用
前往 [Releases 页面](https://github.com/iroha3/onescreen/releases/latest) 下载适合您操作系统的预编译包：
- **Windows**：下载 `onescreen-windows-amd64.zip` 或 `.exe`，解压双击直接运行。
- **macOS**：下载 `onescreen-darwin-arm64.tar.gz`（Apple Silicon）或 `amd64`（Intel），需本地具备 `m1ddc`（`brew install m1ddc`）。
- **Linux**：下载 `onescreen-linux-amd64.tar.gz`，需本地具备 `ddcutil`（`sudo apt install ddcutil`）。

### 2. 选一次对端接口
- 启动后托盘将常驻小屏幕图标。
- 右键点击托盘图标，在 **「另一台接的接口」** 菜单中点选另一台主机所连的输入源（例如 `HDMI 2` 或 `DP 1`）。
- *选错了没关系*：显示器黑屏没切过去表明选错，重新在右键菜单里点选另一个端口即可，改一次永久记住。

### 3. 开始享受
- **手动切屏**：双击托盘图标，或在托盘菜单第一项点击切屏。
- **自动切屏**：开启 Oink 后，在托盘勾选「随 Oink 自动切屏」，鼠标滑向另一台设备时即自动完成切屏。

---

## ❓ 常见问题 (FAQ)

### Q: 频繁自动切屏会搞坏我的显示器吗？
**完全不会。**
1. **全固态电子切换**：现代显示器的信源切换完全由内部主控芯片（Scaler ASIC / Mux）完成，属于纳秒级的时钟重同步与 EDID/HDCP 信号握手，不存在任何继电器或机械磨损部件。
2. **面板与背光不受损**：切换过程中显示器主电源供电轨持续工作，面板不掉电，背光不频繁点火，仅时序控制器（TCON）短暂输出黑场信号防撕裂。
3. **软件层防抖保护**：OneScreen 内置 400ms 边缘缓冲（手滑快速撤回不触发）与 1000ms 冷却机制，且在切换前进行状态幂等检测（已经在目标输入源则跳过），彻底杜绝边缘抖动对显示器主控 NVRAM 的高频写入。

### Q: 我必须在两台设备上都安装 OneScreen 吗？
**绝大多数情况下，只需在一台设备上运行即可！**
- Oink 会在鼠标离开本机时发送失焦通知，在鼠标返回时发送获焦通知。因此一台主机完全具备判定双向切屏的能力。
- *何时建议两台都装？* 少部分显示器固件会在当前非激活端口上休眠 DDC 通道。如果你发现切到另一台后无法自动切回，在两台设备都运行 OneScreen 即可解决（两台机器共用配置文件，自动对调端口）。

### Q: 对端休眠、关机或者连不上了怎么办？
随时按下全局逃生热键 **`Ctrl + Alt + Shift + S`**，OneScreen 将无条件强制发指令将显示器拉回本机。

---

## ⚖️ 开源协议

本项目采用 [GNU Affero General Public License v3.0 (AGPLv3)](LICENSE) 开源协议。  
© 2026 [iroha3](https://github.com/iroha3). All rights reserved.
