param(
    [string]$Version = "0.0.1",
    [string]$OutputDir = "",
    [switch]$SkipAndroid,
    [switch]$SkipAgent
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

if ($Version -notmatch '^\d+\.\d+\.\d+([-.][A-Za-z0-9.-]+)?$') {
    throw "Version must look like 0.0.1"
}

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = Resolve-Path (Join-Path $scriptDir "..")
if ([string]::IsNullOrWhiteSpace($OutputDir)) {
    $OutputDir = Join-Path $repoRoot $Version
}
$releaseDir = [System.IO.Path]::GetFullPath($OutputDir)
$repoRootPath = [System.IO.Path]::GetFullPath($repoRoot)

if (-not $releaseDir.StartsWith($repoRootPath, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "OutputDir must be inside the repository"
}

if (Test-Path -LiteralPath $releaseDir) {
    Remove-Item -LiteralPath $releaseDir -Recurse -Force
}
New-Item -ItemType Directory -Force -Path $releaseDir | Out-Null

$binSource = Join-Path $repoRoot "bin"
$binTarget = Join-Path $releaseDir "bin"
if (-not (Test-Path -LiteralPath $binSource)) {
    throw "Missing dependency bin directory: $binSource"
}
Copy-Item -LiteralPath $binSource -Destination $binTarget -Recurse -Force

$weztermConfig = Join-Path $repoRoot "wezterm-config"
if (Test-Path -LiteralPath $weztermConfig) {
    Copy-Item -LiteralPath $weztermConfig -Destination (Join-Path $releaseDir "wezterm-config") -Recurse -Force
}

$agentConfigExample = Join-Path $repoRoot "agent\config.example.json"
if (Test-Path -LiteralPath $agentConfigExample) {
    New-Item -ItemType Directory -Force -Path (Join-Path $releaseDir "agent") | Out-Null
    Copy-Item -LiteralPath $agentConfigExample -Destination (Join-Path $releaseDir "agent\config.example.json") -Force
}

$agentTray = Join-Path $repoRoot "agent\tray.ps1"
if (Test-Path -LiteralPath $agentTray) {
    New-Item -ItemType Directory -Force -Path (Join-Path $releaseDir "agent") | Out-Null
    Copy-Item -LiteralPath $agentTray -Destination (Join-Path $releaseDir "agent\tray.ps1") -Force
}

if (-not $SkipAgent) {
    $agentExe = Join-Path $releaseDir "EasyCodex.exe"
    Push-Location (Join-Path $repoRoot "agent")
    try {
        $env:GOOS = "windows"
        $env:GOARCH = "amd64"
        go build -trimpath -ldflags "-s -w -H windowsgui -X main.version=$Version" -o $agentExe .\cmd\easycodex-agent
    } finally {
        Pop-Location
        Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
        Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
    }
}

if (-not $SkipAndroid) {
    $androidDir = Join-Path $repoRoot "android"
    $gradlew = Join-Path $androidDir "gradlew.bat"
    $localGradle = Join-Path $androidDir ".gradle-local\gradle-6.7.1\bin\gradle.bat"
    if (Test-Path -LiteralPath $gradlew) {
        $gradleCommand = $gradlew
    } elseif (Test-Path -LiteralPath $localGradle) {
        $gradleCommand = $localGradle
    } else {
        $gradleCommand = "gradle"
    }

    Push-Location $androidDir
    try {
        & $gradleCommand assembleDebug
    } finally {
        Pop-Location
    }

    $apkSource = Join-Path $androidDir "app\build\outputs\apk\debug\app-debug.apk"
    if (-not (Test-Path -LiteralPath $apkSource)) {
        throw "APK was not produced: $apkSource"
    }
    Copy-Item -LiteralPath $apkSource -Destination (Join-Path $releaseDir "EasyCodex-$Version.apk") -Force
}

$manifest = [ordered]@{
    name = "EasyCodex"
    version = $Version
    builtAt = (Get-Date).ToString("o")
    files = @(
        "EasyCodex.exe",
        "EasyCodex-$Version.apk",
        "bin/",
        "wezterm-config/",
        "agent/config.example.json",
        "agent/tray.ps1"
    )
    exeName = "EasyCodex.exe"
    updatableFiles = @(
        "EasyCodex.exe",
        "wezterm-config/"
    )
}
$manifest | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $releaseDir "manifest.json") -Encoding UTF8

Write-Host "Release package created: $releaseDir"
