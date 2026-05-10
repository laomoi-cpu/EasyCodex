# EasyCodex

EasyCodex 是一个面向 Codex 日常工作的 PC + Android 远程终端工作台。

它的核心目标很直接：你主要在 PC 上用 WezTerm GUI 工作；离开电脑时，手机可以无缝接上同一批 Codex 会话，看输出、发指令、启动新会话；回到电脑后，仍然回到原来的 WezTerm GUI 继续工作。

## 产品亮点

- **PC 优先，不牺牲桌面体验**  
  Agent 启动后默认打开 WezTerm GUI。手机只是远程控制器，不会把 Codex 会话藏到后台，也不会打断你回到 PC 后继续操作。

- **同一套会话，手机和 PC 同步查看**  
  Android 端通过 Agent 读取 WezTerm pane snapshot，可以看到当前窗口、tab、pane、工作目录和终端输出。手机输入的命令会发送到同一个 WezTerm pane。

- **手机端支持中文输入和 Enter 提交**  
  Android 发送文本时使用 UTF-8 base64，避免 Windows 终端编码问题。默认发送后自动追加 Enter，适合直接对 Codex 输入中文问题。

- **彩色终端显示**  
  支持 WezTerm 的 ANSI 转义输出，Android 端可以显示 Codex 状态、路径、模型、上下文等彩色文本。设置里可以切换是否显示终端颜色。

- **二维码配对，减少手动配置**  
  PC Agent 提供配对页面和二维码。Android 扫码后自动写入服务器地址、Token 和默认工作目录/启动命令等配置。

- **局域网和公网地址都能配对**  
  Agent 可以生成局域网二维码，也可以配置公网地址，例如 Tailscale IP，方便手机在不同网络环境下连接。

- **托盘常驻和网页配置**  
  Agent 启动后在 Windows 托盘显示图标。右键菜单可以打开设置页面，在网页里配置监听地址、Token、默认工作目录、Codex 启动命令、实例、自动启动等选项。

- **安全策略清楚**  
  默认只监听 `127.0.0.1:8765`，手机访问前需要显式改成 `0.0.0.0:8765` 或配置可访问地址。除健康检查外，API 使用 Token 认证。

- **发布构建可自动化**  
  提供发布脚本和 GitHub Actions workflow，可以生成带版本号的 Windows Agent、Android APK 和依赖 `bin` 目录。

## 典型使用方式

1. 在 PC 上启动 EasyCodex Agent。
2. Agent 自动打开或复用 WezTerm GUI。
3. 在 PC 上正常使用 Codex。
4. 离开电脑时，打开 Android APK。
5. 扫描 PC 配对二维码或选择历史连接。
6. 在手机上查看 pane 输出，输入中文问题或命令。
7. 回到电脑后，继续在原来的 WezTerm GUI 中工作。

## 项目结构

```text
agent/          Windows Agent，提供托盘、网页设置、配对页面和 HTTP API
android/        Android 控制端 APK
bin/            WezTerm 和运行依赖
wezterm-config/ WezTerm 配置和快捷脚本
scripts/        发布构建脚本
```

## 快速运行

启动 PC Agent：

```cmd
agent\bin\easycodex-agent.exe
```

默认监听地址：

```text
http://127.0.0.1:8765
```

如果要让手机在同一 Wi-Fi 下连接，把 `agent\config.json` 里的监听地址改成：

```json
{
  "listen": "0.0.0.0:8765"
}
```

然后确保 Windows 防火墙允许 Agent 访问网络。

## Android 端

Android APK 当前是轻量原生实现，重点是稳定闭环：

- 扫码配对 PC Agent
- 保存最近连接历史，并标记 OK / FAIL / SAVED
- 查看实例、tab、pane
- 轮询终端 snapshot
- 显示彩色终端内容
- 输入中文并发送到 WezTerm
- 支持 Enter、Ctrl+C、Shift+Tab、Shift+PageUp、Shift+PageDown、Space、上下箭头、Esc 等快捷键

## 配置能力

Agent 设置页支持配置：

- 监听地址
- Token
- 是否每次启动重新生成 Token
- 公网 Base URL
- 默认工作目录
- 默认 Codex 启动命令
- WezTerm 实例列表
- 自动启动实例
- 退出 Agent 时是否关闭本次启动的 GUI

这些配置会写入 `agent\config.json`。

## 发布构建

生成 `0.0.1` 发布目录：

```powershell
.\scripts\build-release.ps1 -Version 0.0.1
```

输出结构：

```text
0.0.1/
  EasyCodex-0.0.1.exe
  EasyCodex-0.0.1.apk
  bin/
  wezterm-config/
  agent/config.example.json
  manifest.json
```

GitHub Actions 也支持手动输入版本号构建发布包。

## WezTerm 增强

`wezterm-config\wezterm.lua` 提供了面向 Codex 工作流的增强：

- `Ctrl+V` 粘贴图片时，会保存剪贴板图片到 `captures/` 并把文件路径粘贴到终端
- 无图片时回退到 WezTerm 默认文本粘贴
- 识别 Codex working 状态后，在窗口标题显示 working 会话数量

## 更多文档

- Agent API 和实现细节：`agent\README.md`
- Android 构建说明：`android\README.md`
