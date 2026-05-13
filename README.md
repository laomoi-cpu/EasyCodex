# EasyCodex

最新版本下载：[EasyCodex 0.0.43](https://github.com/laomoi-cpu/EasyCodex/releases/download/v0.0.43/EasyCodex-0.0.43.zip)

EasyCodex 是一个面向 Codex 用户的 Windows 远程终端工作台。

它的核心目标很简单：让你在 PC 上正常用命令行或 CLI 跑 Codex，离开电脑后还能用手机继续查看进度、发送指令、启动新会话；回到电脑时，又能继续在原来的命令行窗口里工作。

目前 EasyCodex **只支持 Windows**。手机 App、PC 浏览器和手机浏览器都是远程控制入口。

## 亮点

- **适合小白用户**  
  启动后自动准备好 PC 端工作环境，不要求用户理解复杂的终端、多路复用、后台进程或网络接口。

- **不破坏 PC 工作习惯**  
  Codex 会话仍然运行在 PC 的命令行窗口里。手机和浏览器只是远程接入同一批会话，离开电脑和回到电脑都能自然衔接。

- **多端都能接上同一套会话**  
  支持 Windows PC、Android App、PC 浏览器和手机浏览器访问同一批 Codex 会话。

- **扫码即可连接**  
  PC 后台提供二维码。手机 App 或浏览器扫码后即可连接，不需要手动复制复杂配置。

- **手机浏览器也有接近 App 的体验**  
  手机网页端针对小屏做了单独布局，终端内容占主要区域，输入区和常用操作靠近底部。

- **能看到有哪些设备接入过**  
  PC 后台可以查看已连接过的浏览器、Android App 和其它客户端，以及最后访问时间。

## 支持平台

当前支持：

- Windows PC
- Android App
- PC 浏览器
- 手机浏览器

当前不支持：

- macOS
- Linux
- iOS 原生 App

iPhone 用户可以先通过手机浏览器访问。

## 使用方式

启动 PC 端：

```cmd
agent\bin\easycodex-agent.exe
```

打开 PC 后台：

```text
http://127.0.0.1:8765/pairing
```

在后台页面扫码连接 Android App 或浏览器终端。

如果要让手机在局域网内访问，请在设置页把监听地址改成：

```text
0.0.0.0:8765
```

并确认 Windows 防火墙允许 EasyCodex 访问网络。

## 技术说明

EasyCodex 底层使用 WezTerm 来启动和管理 Codex 会话。

普通用户可以把它理解成一个更适合远程控制的命令行窗口。相比直接使用 Windows 自带的 cmd，WezTerm 的外观和性能更好，对窗口、标签页、pane、截图和后续扩展功能的支持也更完整。

## 更多文档

- Agent API 和实现细节：`agent\README.md`
- Android 构建说明：`android\README.md`
