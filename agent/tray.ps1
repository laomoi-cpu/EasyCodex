param(
    [Parameter(Mandatory=$true)] [string] $PairingUrl,
    [Parameter(Mandatory=$true)] [string] $ConfigPath,
    [int] $AgentPid = 0
)

Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

function New-EasyCodexIcon {
    $bitmap = New-Object System.Drawing.Bitmap 64, 64
    $graphics = [System.Drawing.Graphics]::FromImage($bitmap)
    $graphics.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
    $rect = New-Object System.Drawing.Rectangle 0, 0, 64, 64
    $brush = New-Object System.Drawing.Drawing2D.LinearGradientBrush $rect, ([System.Drawing.Color]::FromArgb(29, 78, 216)), ([System.Drawing.Color]::FromArgb(14, 165, 233)), 45
    $graphics.FillEllipse($brush, 4, 4, 56, 56)

    $ringPen = New-Object System.Drawing.Pen ([System.Drawing.Color]::FromArgb(130, 219, 234, 254)), 3
    $graphics.DrawEllipse($ringPen, 6, 6, 52, 52)

    $promptPen = New-Object System.Drawing.Pen ([System.Drawing.Color]::White), 6
    $promptPen.StartCap = [System.Drawing.Drawing2D.LineCap]::Round
    $promptPen.EndCap = [System.Drawing.Drawing2D.LineCap]::Round
    $graphics.DrawLine($promptPen, 19, 23, 29, 32)
    $graphics.DrawLine($promptPen, 19, 41, 29, 32)

    $cursorPen = New-Object System.Drawing.Pen ([System.Drawing.Color]::FromArgb(230, 186, 230, 253)), 6
    $cursorPen.StartCap = [System.Drawing.Drawing2D.LineCap]::Round
    $cursorPen.EndCap = [System.Drawing.Drawing2D.LineCap]::Round
    $graphics.DrawLine($cursorPen, 37, 41, 48, 41)
    $icon = [System.Drawing.Icon]::FromHandle($bitmap.GetHicon())
    return @{ Icon = $icon; Bitmap = $bitmap; Graphics = $graphics; Brush = $brush; RingPen = $ringPen; PromptPen = $promptPen; CursorPen = $cursorPen }
}

$baseUrl = $PairingUrl -replace '/pairing$', ''
$settingsUrl = "$baseUrl/settings"
$statusUrl = "$baseUrl/status"
$iconParts = New-EasyCodexIcon

$notify = New-Object System.Windows.Forms.NotifyIcon
$notify.Text = 'EasyCodex Agent'
$notify.Icon = $iconParts.Icon
$notify.Visible = $true

$menu = New-Object System.Windows.Forms.ContextMenuStrip

$pairItem = New-Object System.Windows.Forms.ToolStripMenuItem('Show Pairing QR')
$pairItem.Add_Click({ Start-Process $PairingUrl })
[void]$menu.Items.Add($pairItem)

$configItem = New-Object System.Windows.Forms.ToolStripMenuItem('Settings')
$configItem.Add_Click({
    Start-Process $settingsUrl
})
[void]$menu.Items.Add($configItem)

$statusItem = New-Object System.Windows.Forms.ToolStripMenuItem('Status')
$statusItem.Add_Click({
    Start-Process $statusUrl
})
[void]$menu.Items.Add($statusItem)

[void]$menu.Items.Add((New-Object System.Windows.Forms.ToolStripSeparator))

$exitItem = New-Object System.Windows.Forms.ToolStripMenuItem('Exit EasyCodex')
$exitItem.Add_Click({
    $notify.Visible = $false
    if ($AgentPid -gt 0) {
        Stop-Process -Id $AgentPid -Force -ErrorAction SilentlyContinue
    }
    [System.Windows.Forms.Application]::Exit()
})
[void]$menu.Items.Add($exitItem)

$notify.ContextMenuStrip = $menu
$notify.Add_DoubleClick({ Start-Process $PairingUrl })

$agentTimer = $null
if ($AgentPid -gt 0) {
    $agentTimer = New-Object System.Windows.Forms.Timer
    $agentTimer.Interval = 2000
    $agentTimer.Add_Tick({
        if (-not (Get-Process -Id $AgentPid -ErrorAction SilentlyContinue)) {
            $agentTimer.Stop()
            $notify.Visible = $false
            [System.Windows.Forms.Application]::Exit()
        }
    })
    $agentTimer.Start()
}

[System.Windows.Forms.Application]::Run()

if ($agentTimer -ne $null) {
    $agentTimer.Stop()
    $agentTimer.Dispose()
}
$notify.Dispose()
$iconParts.Icon.Dispose()
$iconParts.Graphics.Dispose()
$iconParts.Brush.Dispose()
$iconParts.RingPen.Dispose()
$iconParts.PromptPen.Dispose()
$iconParts.CursorPen.Dispose()
$iconParts.Bitmap.Dispose()
