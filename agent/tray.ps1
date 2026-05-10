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
    $brush = New-Object System.Drawing.Drawing2D.LinearGradientBrush $rect, ([System.Drawing.Color]::FromArgb(15, 139, 141)), ([System.Drawing.Color]::FromArgb(245, 158, 11)), 45
    $graphics.FillRectangle($brush, $rect)
    $font = New-Object System.Drawing.Font 'Segoe UI', 20, ([System.Drawing.FontStyle]::Bold)
    $textBrush = [System.Drawing.Brushes]::White
    $graphics.DrawString('EC', $font, $textBrush, 10, 15)
    $pen = New-Object System.Drawing.Pen ([System.Drawing.Color]::FromArgb(235, 255, 255, 255)), 4
    $graphics.DrawLine($pen, 14, 48, 50, 48)
    $icon = [System.Drawing.Icon]::FromHandle($bitmap.GetHicon())
    return @{ Icon = $icon; Bitmap = $bitmap; Graphics = $graphics; Brush = $brush; Font = $font; Pen = $pen }
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

$exitItem = New-Object System.Windows.Forms.ToolStripMenuItem('Exit Tray')
$exitItem.Add_Click({
    $notify.Visible = $false
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
$iconParts.Font.Dispose()
$iconParts.Pen.Dispose()
$iconParts.Bitmap.Dispose()
