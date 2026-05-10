$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

$captureDir = "D:\EasyTerm\captures"
New-Item -ItemType Directory -Force -Path $captureDir | Out-Null

if (-not [System.Windows.Forms.Clipboard]::ContainsImage()) {
  exit 0
}

$image = [System.Windows.Forms.Clipboard]::GetImage()
if ($null -eq $image) {
  exit 0
}

try {
  $timestamp = Get-Date -Format "yyyyMMdd-HHmmss-fff"
  $path = Join-Path $captureDir "clipboard-$timestamp.png"
  $image.Save($path, [System.Drawing.Imaging.ImageFormat]::Png)
  [Console]::OutputEncoding = [System.Text.Encoding]::UTF8
  Write-Output $path
} finally {
  $image.Dispose()
}
