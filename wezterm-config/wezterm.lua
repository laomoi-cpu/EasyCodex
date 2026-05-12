local wezterm = require 'wezterm'

local config = wezterm.config_builder()

config.status_update_interval = 1000

local codex_spinner_frames = {
  '⠋',
  '⠙',
  '⠹',
  '⠸',
  '⠼',
  '⠴',
  '⠦',
  '⠧',
  '⠇',
  '⠏',
  '◐',
  '◓',
  '◑',
  '◒',
}

local function contains_literal(value, needle)
  return value and needle and value:find(needle, 1, true) ~= nil
end

local function is_working_pane(pane, extra_title)
  if not pane then
    return false
  end

  local user_vars = pane.user_vars or {}
  if user_vars.codextab_status == 'working' or user_vars.easycodex_status == 'working' then
    return true
  end

  local title = (pane.title or '') .. ' ' .. (extra_title or '')
  local lower_title = title:lower()
  if contains_literal(lower_title, 'working') or contains_literal(lower_title, 'thinking') or contains_literal(lower_title, 'running') then
    return true
  end

  for _, frame in ipairs(codex_spinner_frames) do
    if contains_literal(title, frame) then
      return true
    end
  end

  return false
end

local function working_tab_count(tabs)
  local busy = 0
  for _, item in ipairs(tabs) do
    if is_working_pane(item.active_pane, item.tab_title) then
      busy = busy + 1
    end
  end
  return busy
end

wezterm.on('format-tab-title', function(tab, tabs, panes, config, hover, max_width)
  local title = tab.tab_title
  if not title or #title == 0 then
    title = tab.active_pane.title
  end

  local marker = ''
  if tab.active_pane.has_unseen_output then
    marker = ' *'
  end
  if is_working_pane(tab.active_pane, tab.tab_title) then
    marker = marker .. ' working'
  end

  return string.format(' %d:%s%s ', tab.tab_index + 1, title, marker)
end)

wezterm.on('format-window-title', function(tab, pane, tabs, panes, config)
  local busy = working_tab_count(tabs)

  if busy > 0 then
    return string.format('EasyCodex (%d working) - %d sessions', busy, #tabs)
  end

  return string.format('EasyCodex - %d sessions', #tabs)
end)

return config
