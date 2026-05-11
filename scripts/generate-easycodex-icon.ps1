param(
    [Parameter(Mandatory=$true)] [string] $OutputPath
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

Add-Type -AssemblyName System.Drawing

$fullPath = [System.IO.Path]::GetFullPath($OutputPath)
$parent = [System.IO.Path]::GetDirectoryName($fullPath)
if (-not [System.IO.Directory]::Exists($parent)) {
    [System.IO.Directory]::CreateDirectory($parent) | Out-Null
}

$bitmap = New-Object System.Drawing.Bitmap 256, 256
$graphics = [System.Drawing.Graphics]::FromImage($bitmap)
$graphics.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
$graphics.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
$graphics.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality

$rect = New-Object System.Drawing.Rectangle 0, 0, 256, 256
$brush = New-Object System.Drawing.Drawing2D.LinearGradientBrush $rect, ([System.Drawing.Color]::FromArgb(29, 78, 216)), ([System.Drawing.Color]::FromArgb(14, 165, 233)), 45
$graphics.FillEllipse($brush, 16, 16, 224, 224)

$ringPen = New-Object System.Drawing.Pen ([System.Drawing.Color]::FromArgb(145, 219, 234, 254)), 12
$graphics.DrawEllipse($ringPen, 24, 24, 208, 208)

$promptPen = New-Object System.Drawing.Pen ([System.Drawing.Color]::White), 24
$promptPen.StartCap = [System.Drawing.Drawing2D.LineCap]::Round
$promptPen.EndCap = [System.Drawing.Drawing2D.LineCap]::Round
$graphics.DrawLine($promptPen, 76, 92, 116, 128)
$graphics.DrawLine($promptPen, 76, 164, 116, 128)

$cursorPen = New-Object System.Drawing.Pen ([System.Drawing.Color]::FromArgb(235, 186, 230, 253)), 24
$cursorPen.StartCap = [System.Drawing.Drawing2D.LineCap]::Round
$cursorPen.EndCap = [System.Drawing.Drawing2D.LineCap]::Round
$graphics.DrawLine($cursorPen, 148, 164, 192, 164)

$icon = [System.Drawing.Icon]::FromHandle($bitmap.GetHicon())
$stream = [System.IO.File]::Open($fullPath, [System.IO.FileMode]::Create, [System.IO.FileAccess]::Write)
try {
    $icon.Save($stream)
} finally {
    $stream.Dispose()
    $icon.Dispose()
    $graphics.Dispose()
    $brush.Dispose()
    $ringPen.Dispose()
    $promptPen.Dispose()
    $cursorPen.Dispose()
    $bitmap.Dispose()
}
