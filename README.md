<div align="center">

<img src="tray.png" width="80" height="80" alt="OneScreen Icon" />

# OneScreen

**单显示器多机极简切屏工具**

一块显示器，两台主机。双击托盘即切设备；配合 [Oink](https://github.com/oinkvm/oink) 更可实现鼠标划过屏幕边缘瞬间自动切屏。

[![GitHub release](https://img.shields.io/github/v/release/iroha3/onescreen?color=orange&label=Release)](https://github.com/iroha3/onescreen/releases/latest)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20macOS%20%7C%20Linux-blue)]()
[![License: AGPL v3](https://img.shields.io/badge/License-AGPLv3-green.svg)](LICENSE)

[下载最新版本](https://github.com/iroha3/onescreen/releases/latest) · [产品主页](https://iroha3.github.io/onescreen/) · [常见问题](#常见问题)

</div>

---

## 方案对比

| 方案 | 硬件成本 | 布线复杂度 | 切换方式 | 信号链路与画质 |
| :--- | :--- | :--- | :--- | :--- |
| **物理 KVM 切换器** | 较高 | 线缆数量增加 | 实体按键或线控 | 信号经过中转，存在兼容性或带宽受限风险 |
| **显示器 OSD 按键** | 无 | 无 | 繁琐难按 | 主机直连，按键损耗 |
| **OneScreen** | 开源免费 | 无 | 托盘双击 / 鼠标边缘自动触发 | 主机直连，原生高刷无损 |

---

## 核心特性

- **鼠标穿越，信源同行**：联动 [Oink](https://github.com/oinkvm/oink) 自动切屏，鼠标跨屏时自动切换显示器输入源，体验丝滑如单一大桌面。
- **支持单机部署**：在显示器允许的情况下，仅需在一台主机运行即可响应双向事件，控制两端信源切换。
- **防抖与冷却机制**：内置 400ms 边缘缓冲与 1000ms 冷却间隔，过滤边缘误触并避免高频指令下发。
- **强制切回热键**：提供全局快捷键 `Ctrl + Alt + Shift + S`，在对端休眠、死机或无信号时可强制将显示器切回本机。
- **配置跨机自适应**：双机部署时可直接复用同一份配置文件，程序依据主机名自动对调「本机接口」与「对端接口」。
- **低资源占用**：单文件免安装运行，内存占用低于 10MB，常驻系统托盘，支持简体中文与英文界面。

---

## 快速开始

### 1. 下载与依赖

前往 [Releases 页面](https://github.com/iroha3/onescreen/releases/latest) 获取对应平台的预编译包：

- **Windows**：下载 `onescreen-windows-amd64.zip` 或 `.exe`，解压后直接运行，无额外依赖。
- **macOS**：下载 `onescreen-darwin-arm64.tar.gz`（Apple Silicon）或 `amd64`（Intel），需预先安装 `m1ddc`（`brew install m1ddc`）。
- **Linux**：下载 `onescreen-linux-amd64.tar.gz`，需预先安装 `ddcutil`（例如 `sudo apt install ddcutil`）。

### 2. 配置对端接口

1. 启动程序，系统托盘将显示 OneScreen 图标。
2. 右键点击图标，在 **「另一台接的接口」** 菜单中选择对端主机对应的输入源（例如 `HDMI 2` 或 `DP 1`）。
3. 配置会自动持久化。若误选导致黑屏无信号，重新选择正确端口即可。

### 3. 切换方式

- **手动切换**：双击托盘图标，或点击托盘菜单第一项。
- **自动切换**：启动 Oink 后，在托盘菜单中勾选「随 Oink 自动切屏」，鼠标移向对端时即可自动触发切换。

---

## 常见问题

### 频繁切换输入源是否影响显示器寿命？

不会。显示器信源切换由内部 Scaler 芯片固态选通，无继电器等机械磨损；切屏期间面板与主电源持续供电，背光不熄灭。软件内建 400ms 缓冲防抖与 1000ms 冷却，杜绝反复写入。

### 是否需要在两台主机上同时安装？

启用随 Oink 自动切屏功能后，只需要安装在一台主机上。因为 Oink 全局双向通知失焦与获焦事件，单机已完整具备往返判定能力。

---

## 开源协议

本项目基于 [GNU Affero General Public License v3.0 (AGPLv3)](LICENSE) 协议开源。  

© 2026 [iroha3](https://github.com/iroha3). All rights reserved.
