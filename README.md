# EasyCodex

EasyCodex 是一个面向 Codex 用户的 Windows 远程终端工作台。

它解决的是一个很实际的问题：你主要在 Windows 电脑上用命令行或 CLI 跑 Codex，离开电脑后，又希望能用手机继续查看进度、发送指令、启动新会话；回到电脑时，还能继续在原来的命令行窗口里工作。

目前 EasyCodex **只支持 Windows**。PC 端使用命令行窗口运行 Codex，手机 App、PC 浏览器和手机浏览器都是远程控制入口。

## 产品亮点

- **操作简单，适合小白用户**  
  启动 EasyCodex Agent 后，它会自动打开或复用 WezTerm GUI。用户不需要理解复杂的终端、多路复用、后台进程或网络接口。

- **PC 工作体验不被破坏**  
  Codex 会话默认运行在 PC 上的 WezTerm GUI 里。手机和浏览器只是接入同一批会话，不会把会话藏到后台。

- **手机离开电脑也能继续看进度**  
  手机 App 或手机浏览器可以查看 Codex 输出、发送中文指令、切换 pane、启动新会话。

- **支持多端登录使用**  
  同一套会话可以从 Windows PC、PC 浏览器、手机浏览器和 Android App 访问。

- **扫码连接，不用手抄配置**  
  PC 后台配对页会生成二维码。Android App 和浏览器终端扫码后，会自动带上 Agent 地址和 Token。

- **首次启动更安全**  
  第一次启动 Agent 时，如果配置文件不存在，会自动生成随机 Token 并写入 `agent/config.json`，不再使用固定默认 Token。

- **支持中文输入和彩色终端**  
  手机 App 和网页终端都支持中文输入，也支持显示大部分 ANSI 彩色终端输出。

- **可查看已连接终端**  
  PC 后台提供 Connections 页面，可以看到当前接入过的浏览器、Android App 和其它 API 客户端，以及最后访问时间。

## 支持平台

当前支持：

- Windows PC Agent
- Windows 上的 WezTerm GUI
- PC 浏览器终端
- 手机浏览器终端
- Android App

当前不支持：

- macOS Agent
- Linux Agent
- iOS 原生 App

iPhone 用户可以先通过手机浏览器访问。

## 典型使用方式

1. 在 Windows 电脑上启动 EasyCodex Agent。
2. Agent 自动打开或复用 WezTerm GUI。
3. 在 PC 上正常使用 Codex。
4. 离开电脑时，用 Android App 或手机浏览器扫码连接。
5. 在手机上查看 Codex 输出，继续输入问题或命令。
6. 回到电脑后，继续在原来的 WezTerm GUI 中工作。

## 主要功能

- Windows 托盘常驻
- 网页设置页面
- 二维码配对页面
- Android App 远程终端
- PC 浏览器远程终端
- 手机浏览器远程终端
- 手机浏览器类 App 布局
- 连接终端列表和最后访问时间
- 局域网连接
- Tailscale 等公网地址连接
- 首次启动随机 Token
- 可选每次启动重新生成 Token
- 中文输入
- 彩色终端显示
- 新建 Codex 会话
- 删除会话
- 克隆会话
- Codex working 状态显示

## 快速运行

启动 PC Agent：

```cmd
agent\bin\easycodex-agent.exe
```

默认本机地址：

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

## PC 后台页面

Agent 启动后，可以在 PC 浏览器打开：

```text
http://127.0.0.1:8765/pairing
```

常用页面：

- `/pairing`：二维码配对页面
- `/connections`：查看已经连接过的浏览器、Android App 和 API 客户端
- `/settings`：配置监听地址、Token、默认工作目录、Codex 启动命令等
- `/status`：查看 Agent 状态

后台导航里不会直接显示 Terminal 链接，避免把管理后台和终端入口混在一起。

## 浏览器终端

PC 浏览器和手机浏览器都可以访问：

```text
http://<电脑IP>:8765/terminal
```

更推荐从 PC 的 `/pairing` 页面扫码进入。二维码会自动携带服务器地址和 Token。

手机浏览器终端会使用更接近 Android App 的布局：

- 顶部显示连接状态和会话列表
- 中间大面积显示终端内容
- 底部固定输入框和发送按钮
- 特殊按键默认折叠，需要时再展开

## Android App

Android App 适合手机常用场景：

- 扫码连接 PC Agent
- 查看会话列表
- 查看终端输出
- 输入中文命令
- 使用 Enter、Ctrl+C、Esc、上下箭头、Shift+PageUp、Shift+PageDown 等快捷键
- 新建、删除、克隆 Codex 会话

## 发布构建

生成发布包：

```powershell
.\scripts\build-release.ps1 -Version 0.0.3
```

输出结构示例：

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

## 技术说明

EasyCodex 底层使用 WezTerm 来启动和管理 Codex 会话。

普通用户可以把它理解成一个更适合远程控制的命令行窗口。相比直接使用 Windows 自带的 cmd，WezTerm 的外观和性能更好，对彩色终端输出、窗口、标签页、pane、截图和后续扩展功能的支持也更完整。

## 更多文档

- Agent API 和实现细节：`agent\README.md`
- Android 构建说明：`android\README.md`
