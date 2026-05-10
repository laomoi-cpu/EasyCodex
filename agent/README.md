# EasyTerm Agent

EasyTerm Agent 是运行在 Windows PC 上的 HTTP 控制服务。Android 控制器连接 Agent，Agent 再通过 WezTerm CLI 控制指定的 WezTerm 实例。

## 运行

```cmd
D:\EasyTerm\tools\go\bin\go.exe run .\cmd\easyterm-agent
```

默认监听：

```text
http://127.0.0.1:8765
```

如果 `agent\config.json` 不存在，Agent 会使用内置默认配置，并在启动时生成一个临时 token。

## API

```http
GET  /api/health
GET  /api/instances
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
