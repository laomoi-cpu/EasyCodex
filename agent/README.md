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
  "text": "dir\r"
}
```

启动实例：

```http
POST /api/instances/main/launch
```

Agent 会启动：

```cmd
bin\wezterm-gui.exe start --class easycodex
```

并自动设置 `WEZTERM_CONFIG_FILE` 指向 `wezterm-config\wezterm.lua`。
