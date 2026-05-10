# EasyTerm

Portable WezTerm-based terminal customization workspace with a local Agent API.

## Agent

Start the PC Agent:

```cmd
D:\EasyTerm\agent\bin\easyterm-agent.exe
```

Default address:

```text
http://127.0.0.1:8765
```

Instance parameters are controlled by `agent\config.json`. If that file does not exist, the Agent uses
the default instance:

```json
{
  "id": "main",
  "name": "主终端",
  "class": "easyterm"
}
```

Launch the configured WezTerm instance through the Agent:

```http
POST /api/instances/main/launch
```

The Agent starts `bin\wezterm-gui.exe start --class easyterm` and sets `WEZTERM_CONFIG_FILE` to
`wezterm-config\wezterm.lua`.

See `agent\README.md` for the HTTP API.

## Image Paste

`Ctrl+V` is customized in `wezterm-config\wezterm.lua`.

- If the Windows clipboard contains an image, `scripts\paste-image.ps1` saves it under `D:\EasyTerm\captures` and pastes the quoted image path into the active pane.
- If the clipboard does not contain an image, EasyTerm falls back to WezTerm's normal text paste.

`captures\` is ignored by git.
