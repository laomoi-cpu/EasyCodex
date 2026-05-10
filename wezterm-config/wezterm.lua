local wezterm = require 'wezterm'
local act = wezterm.action

local config = wezterm.config_builder()

local easycodex_root = os.getenv 'EASYCODEX_ROOT' or 'D:\\EasyCodex'
local paste_image_script = easycodex_root .. '\\wezterm-config\\scripts\\paste-image.ps1'

local function trim(value)
  return (value or ''):gsub('%s+$', '')
end

local function quote_for_shell(path)
  return '"' .. path:gsub('"', '\\"') .. '"'
end

local paste_image_or_clipboard = wezterm.action_callback(function(window, pane)
  local ok, stdout, stderr = wezterm.run_child_process {
    'powershell.exe',
    '-NoProfile',
    '-Sta',
    '-ExecutionPolicy',
    'Bypass',
    '-File',
    paste_image_script,
  }

  local path = trim(stdout)
  if ok and path ~= '' then
    pane:send_paste(quote_for_shell(path) .. ' ')
    return
  end

  if not ok and stderr and stderr ~= '' then
    wezterm.log_warn('paste-image.ps1 failed: ' .. stderr)
  end

  window:perform_action(act.PasteFrom 'Clipboard', pane)
end)

config.keys = {
  { key = 'v', mods = 'CTRL', action = paste_image_or_clipboard },
  { key = 'V', mods = 'CTRL', action = paste_image_or_clipboard },
  { key = 'v', mods = 'SHIFT|CTRL', action = paste_image_or_clipboard },
  { key = 'V', mods = 'SHIFT|CTRL', action = paste_image_or_clipboard },
}

config.status_update_interval = 1000

wezterm.on('format-tab-title', function(tab, tabs, panes, config, hover, max_width)
  local title = tab.tab_title
  if not title or #title == 0 then
    title = tab.active_pane.title
  end

  local marker = ''
  if tab.active_pane.has_unseen_output then
    marker = ' *'
  end

  return string.format(' %d:%s%s ', tab.tab_index + 1, title, marker)
end)

wezterm.on('format-window-title', function(tab, pane, tabs, panes, config)
  local busy = 0
  for _, item in ipairs(tabs) do
    local active_pane = item.active_pane
    if active_pane and active_pane.user_vars and active_pane.user_vars.codextab_status == 'working' then
      busy = busy + 1
    end
  end

  if busy > 0 then
    return string.format('EasyCodex - %d working / %d sessions', busy, #tabs)
  end

  return string.format('EasyCodex - %d sessions', #tabs)
end)

return config
