param(
    [Parameter(Mandatory=$true)] [string] $PairingUrl,
    [Parameter(Mandatory=$true)] [string] $ConfigPath
)

Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

$notify = New-Object System.Windows.Forms.NotifyIcon
$notify.Text = 'EasyCodex Agent'
$notify.Icon = [System.Drawing.SystemIcons]::Application
$notify.Visible = $true

$menu = New-Object System.Windows.Forms.ContextMenuStrip

$pairItem = New-Object System.Windows.Forms.ToolStripMenuItem('Show Pairing QR')
$pairItem.Add_Click({ Start-Process $PairingUrl })
[void]$menu.Items.Add($pairItem)

$configItem = New-Object System.Windows.Forms.ToolStripMenuItem('Open Config')
$configItem.Add_Click({
    if (Test-Path -LiteralPath $ConfigPath) {
        Start-Process notepad.exe -ArgumentList @($ConfigPath)
    } else {
        [System.Windows.Forms.MessageBox]::Show("Config file not found:`n$ConfigPath", 'EasyCodex Agent') | Out-Null
    }
})
[void]$menu.Items.Add($configItem)

$statusItem = New-Object System.Windows.Forms.ToolStripMenuItem('Open Local Status')
$statusItem.Add_Click({
    $statusUrl = $PairingUrl -replace '/pairing$', '/api/health'
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

[System.Windows.Forms.Application]::Run()

$notify.Dispose()