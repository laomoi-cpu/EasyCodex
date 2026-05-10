# EasyCodex Agent

EasyCodex Agent 是运行在 Windows PC 上的 HTTP 控制服务。Android 控制器连接 Agent，Agent 再通过 WezTerm CLI 控制指定的 WezTerm 实例。

## 运行

```cmd
D:\EasyCodex\tools\go\bin\go.exe run .\cmd\easycodex-agent
```

默认监听：

```text
http://127.0.0.1:8765
```

如果 `agent\config.json` 不存在，Agent 会使用内置默认配置，并在启动时生成一个临时 token。

默认配置会自动启动 `main` 实例。要关闭自动启动，在 `agent\config.json` 中设置：

```json
{
  "autoLaunch": []
}
```

Agent 自动启动前会先检查对应 WezTerm class 是否已有会话；已有会话时不会重复启动新窗口。

默认情况下，关闭 Agent 不会关闭 WezTerm GUI，避免误杀正在使用的终端。测试或托管模式可以开启：

```json
{
  "closeLaunchedGuiOnExit": true
}
```

该选项只尝试关闭本次 Agent 启动后记录到的 GUI 进程。

## API

```http
GET  /api/health
GET  /api/instances
POST /api/instances/{instanceId}/launch
GET  /api/instances/{instanceId}/sessions
GET  /api/instances/{instanceId}/panes/{paneId}/text?lines=200
POST /api/instances/{instanceId}/panes/{paneId}/send
POST /api/instances/{instanceId}/spawn
```

除 `/api/health` 外，其它接口需要 token：

```http
Authorization: Bearer <token>
```

发送命令示例：

```json
{
  "text": "dir",
  "enter": true
}
```

`enter: true` 会把回车作为独立按键发送，适合 Codex 这类 TUI 程序。需要发送中文时，推荐传 UTF-8 base64，避免 Windows 客户端编码差异：

```json
{
  "textBase64": "5YiG5p6Q6aG555uu55uu5b2V",
  "enter": true
}
```

默认会在发送文本后等待 100ms 再发送回车，避免 TUI 程序还没处理完输入。可以用 `enterDelayMillis` 覆盖，最大 2000ms。

启动实例：

```http
POST /api/instances/main/launch
```

Agent 会启动：

```cmd
bin\wezterm-gui.exe start --class easycodex
```

并自动设置 `WEZTERM_CONFIG_FILE` 指向 `wezterm-config\wezterm.lua`。
