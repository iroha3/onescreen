# OneScreen

单显示器多设备切换。不用硬件 KVM，不用装第三方 DDC 工具，不用记 VCP 数字。

双击托盘 = 切到另一台。配合 [Oink](https://github.com/oinkvm/oink) 还能跟着鼠标焦点自动切。

## 编译与跨平台

### Windows
```bash
# 换图标或清单时才需要重跑这一行（生成 PE 资源，exe 图标和现代控件清单靠它）
rsrc -manifest app.manifest -ico icon.ico -o rsrc_windows_amd64.syso -arch amd64

go build -ldflags "-H windowsgui -s -w" -o onescreen.exe .
```

### Linux (Ubuntu / Debian / Fedora)
```bash
go build -ldflags "-s -w" -o onescreen .
```
Linux 下通过 `ddcutil` 控制显示器（`sudo apt install ddcutil`），关于弹窗原生支持 `zenity` 与 `kdialog`。

### macOS (Apple Silicon / Intel)
```bash
go build -ldflags "-s -w" -o onescreen .
```
macOS 默认支持 AppleScript 对话框与 LaunchAgents 自启；DDC 控制支持 `m1ddc`（Apple Silicon 首选：`brew install m1ddc`）或 `ddcctl`（Intel：`brew install ddcctl`）。

## 用法

双击 `onescreen.exe`，托盘出现复古监视器图标。

- **双击托盘** → 切到对端
- **应急热键** → `Ctrl + Alt + Shift + S` 无论何时强制切回本机（防对端锁屏/假死）
- **右键** → 菜单

```
切到另一台 (HDMI 2)             ← 显示目标接口友好名称
随 Oink 自动切屏 ▸              ← 未连接时变灰提示，连上可用
   启用 ✓
   延迟 ▸ 200 / 300 / 400 / 600 / 800 / 1000 ms
─────
另一台接的接口 ▸ HDMI 1 (17) / HDMI 2 (18)   已排除本机口
─────
开机自启
帮助/关于…
退出
```

**第一次用**：另一台接的接口默认取候选里最小的那个。切过去黑一下屏说明选错了，在菜单里换一个就行，
改一次永久记住。

菜单和关于窗口跟随系统语言（中/英），换语言重启生效。

## 零配置是怎么做到的

| 需要的东西 | 从哪来 |
| --- | --- |
| Oink 装在哪 | `ipc.json` 里的 pid → 反查进程镜像路径 → 卸载项 `InstallLocation` → 自启项 → 常见路径 |
| 显示器叫什么 | 从注册表里的 EDID 解析（0xFC 描述符） |
| 显示器支持哪些输入 | 显示器自己的能力串 `vcp(... 60(11 12 0F) ...)`，十六进制 |
| 本机接的是哪个口 | 读一次 VCP 0x60。你能点到托盘，说明屏幕正显示这台机器，读到的就是自己的口 |
| 对端接的是哪个口 | 候选里默认取最小的，选错了换一个（切过去黑屏就是选错了） |

## 单实例

用命名互斥体保证只有一个在跑——两个实例同时写 DDC 会打架。再启动一次会提示"已经在运行"。

## 两台机器都装

每台都装，配置各存各的（在 exe 同级）。两台同时开自动切不会打架：
切换走的是「先读当前口，已经是目标就不写」，谁先到谁写，后到的直接跳过。

把整个文件夹拷到另一台机器也能直接用——配置里记了机器名，发现不是本机会自动把两个口对调。

## 文件

`config.json` 和 `switch.log` 都在 **exe 同级目录**，整个文件夹拷走就能用。
放在 `Program Files` 这类不可写的位置时，自动退回 `%APPDATA%\OneScreen`。

## 命令行调试

```bash
onescreen.exe --ddc-list           # 列出显示器 + 支持的输入源
onescreen.exe --ddc-caps           # 打印原始能力串
onescreen.exe --monitor-name       # 打印显示器枚举与 EDID 解析结果
onescreen.exe --ddc-get 0          # 读输入源（VCP 0x60）
onescreen.exe --ddc-set 0 18       # 写输入源
onescreen.exe --ddc-get 0 0x10     # 读任意 VCP，如亮度
```

程序是 GUI 子系统的，启动时会先检查有没有可用的 stdout，没有才附加到调用方的控制台。
输出是英文的：终端默认按 GBK 解码，中文会变乱码。

写输入源后立刻回读，有些显示器会返回垃圾值（`invalid value in its command field`），
等一两秒再读。写入本身是成功的，不要用回读来判断切换是否成功。

## 排错

- 菜单顶部出现「显示器没有响应 DDC/CI」：线材/扩展坞挡了 DDC，或显示器关了 DDC/CI。
- `[switch] 已经在对端，跳过` 是正常的：说明当前口已经是目标，没重复写。
- 日志里 `[oink]` 找不到安装：Oink 可能装在非标准位置，且没写注册表。
