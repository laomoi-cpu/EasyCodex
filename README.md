# EasyCodex

EasyCodex 是一个给 Codex 用户准备的 Windows 远程终端工作台。

它的目标是让使用 Codex 变得更简单：在 Windows 电脑上启动一次 EasyCodex，就可以在电脑本机、手机、PC 浏览器、手机浏览器和 Android App 里查看同一批 Codex 会话，继续输入问题、看输出、启动新会话。

目前 EasyCodex **只支持 Windows**。PC 端会使用 WezTerm GUI，手机和浏览器端作为远程控制入口。

## 为什么适合小白用户

- **启动简单**  
  在 Windows 上运行 EasyCodex Agent 后，它会自动打开或复用 WezTerm GUI，不需要用户理解复杂的终端、多路复用或后台进程。

- **扫码即可连接手机**  
  PC 上会提供二维码。手机浏览器或 Android App 扫码后，会自动带上服务器地址和 Token，不需要手动复制一长串配置。

- **首次启动更安全**  
  第一次启动 Agent 时会自动生成随机 Token 并写入配置文件，不再使用固定默认 Token。

- **多端入口统一**  
  同一套 Codex 会话可以从多个入口访问：
  - Windows PC 上的 WezTerm GUI
  - PC 浏览器
  - 手机浏览器
  - Android App

- **不用担心回到电脑接不上**  
  EasyCodex 默认把 Codex 会话开在 PC 的 WezTerm GUI 里。你离开电脑时用手机查看，回到电脑后仍然能在原窗口继续工作。

- **中文输入可用**  
  手机端和网页端都支持输入中文并发送到 Codex，适合直接用中文描述需求。

- **终端内容更容易看**  
  支持彩色终端输出，可以看到 Codex 状态、路径、模型、上下文等颜色提示。

## 支持的平台

当前支持：

- Windows PC
- Android 手机 App
- PC 浏览器
- 手机浏览器

当前不支持：

- macOS Agent
- Linux Agent
- iOS 原生 App

iPhone 用户可以先通过手机浏览器访问。

## 典型使用流程

1. 在 Windows 电脑上启动 EasyCodex Agent。
2. Agent 自动打开 WezTerm GUI。
3. 在 PC 上正常使用 Codex。
4. 离开电脑时，用手机浏览器或 Android App 扫描二维码。
5. 手机上继续查看 Codex 输出，输入问题或命令。
6. 回到电脑后，继续在原来的 WezTerm GUI 里工作。

## 主要功能

- Windows 托盘常驻
- 网页设置页面
- 二维码配对
- 局域网连接
- Tailscale 等公网地址连接
- PC 浏览器终端
- 手机浏览器终端
- Android App 终端
- 彩色终端显示
- 中文输入
- 新建 Codex 会话
- 删除会话
- 克隆会话
- Codex working 状态显示

## 快速运行

启动 PC Agent：

```cmd
agent\bin\easycodex-agent.exe
```

默认地址：

```text
http://127.0.0.1:8765
```

如果要让手机连接，请在设置页或 `agent\config.json` 中把监听地址改成：

```json
{
  "listen": "0.0.0.0:8765"
}
```

同时确认 Windows 防火墙允许 EasyCodex Agent 访问网络。

## 浏览器访问

PC 浏览器和手机浏览器都可以打开：

```text
http://<电脑IP>:8765/terminal
```

更推荐从 PC 的配对页面扫码进入：

```text
http://127.0.0.1:8765/pairing
```

配对页会分别提供 Android App 和浏览器端二维码。

## Android App

Android App 适合手机常用场景：

- 扫码连接 PC
- 查看会话列表
- 查看终端输出
- 输入中文命令
- 使用 Enter、Ctrl+C、Esc、上下箭头等快捷键
- 新建、删除、克隆 Codex 会话

## 发布构建

生成发布包：

```powershell
.\scripts\build-release.ps1 -Version 0.0.3
```

输出结构：

```text
0.0.3/
  EasyCodex-0.0.3.exe
  EasyCodex-0.0.3.apk
  bin/
  wezterm-config/
  agent/config.example.json
  manifest.json
```

GitHub Actions 支持推送 `v0.0.x` 标签后自动构建发布包。

## 更多文档

- Agent API 和实现细节：`agent\README.md`
- Android 构建说明：`android\README.md`
