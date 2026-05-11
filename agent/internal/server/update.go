package server

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"easycodex-agent/internal/winproc"
)

var githubLatestReleaseURL = "https://api.github.com/repos/laomoi-cpu/EasyCodex/releases/latest"
var updateHTTPClient = &http.Client{Timeout: 45 * time.Second}

type updateCheckResponse struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	CanUpdate      bool   `json:"canUpdate"`
	UpToDate       bool   `json:"upToDate"`
	IsDev          bool   `json:"isDev"`
	Message        string `json:"message"`
	ReleaseURL     string `json:"releaseUrl"`
	ZipURL         string `json:"zipUrl"`
	PublishedAt    string `json:"publishedAt"`
}

type updateApplyResponse struct {
	Started        bool   `json:"started"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	Message        string `json:"message"`
}

type githubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	HTMLURL     string        `json:"html_url"`
	PublishedAt string        `json:"published_at"`
	Draft       bool          `json:"draft"`
	Prerelease  bool          `json:"prerelease"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func (s *Server) checkUpdate(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	info, err := checkLatestUpdate(ctx, AppVersion)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeOK(w, http.StatusOK, info)
}

func (s *Server) applyUpdate(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	info, err := checkLatestUpdate(ctx, AppVersion)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if !info.CanUpdate {
		writeError(w, http.StatusBadRequest, errors.New(info.Message))
		return
	}

	cfg := s.configSnapshot()
	if err := prepareAndStartUpdater(ctx, cfg.Root, info); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeOK(w, http.StatusAccepted, updateApplyResponse{
		Started:        true,
		CurrentVersion: info.CurrentVersion,
		LatestVersion:  info.LatestVersion,
		Message:        "Update prepared. Agent will restart automatically.",
	})
	go func() {
		time.Sleep(600 * time.Millisecond)
		os.Exit(0)
	}()
}

func checkLatestUpdate(ctx context.Context, current string) (updateCheckResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubLatestReleaseURL, nil)
	if err != nil {
		return updateCheckResponse{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "EasyCodex-Agent")

	res, err := updateHTTPClient.Do(req)
	if err != nil {
		return updateCheckResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return updateCheckResponse{}, fmt.Errorf("GitHub release check failed: %s", res.Status)
	}

	var release githubRelease
	if err := json.NewDecoder(res.Body).Decode(&release); err != nil {
		return updateCheckResponse{}, err
	}
	latest := normalizeVersion(release.TagName)
	zipURL := releaseZipURL(release, latest)
	info := updateCheckResponse{
		CurrentVersion: current,
		LatestVersion:  latest,
		ReleaseURL:     release.HTMLURL,
		ZipURL:         zipURL,
		PublishedAt:    release.PublishedAt,
		IsDev:          isDevVersion(current),
	}
	if release.Draft || release.Prerelease {
		info.Message = "Latest release is not a stable public version."
		return info, nil
	}
	if latest == "" {
		info.Message = "Latest release does not contain a valid version."
		return info, nil
	}
	if zipURL == "" {
		info.Message = "Latest release does not contain an EasyCodex zip package."
		return info, nil
	}
	if info.IsDev {
		info.CanUpdate = true
		info.Message = "Current build is dev. You can install the latest release."
		return info, nil
	}
	cmp := compareVersions(current, latest)
	info.UpToDate = cmp >= 0
	info.CanUpdate = cmp < 0
	if info.UpToDate {
		info.Message = "Already up to date."
	} else {
		info.Message = "New version available."
	}
	return info, nil
}

func releaseZipURL(release githubRelease, version string) string {
	want := "EasyCodex-" + version + ".zip"
	for _, asset := range release.Assets {
		if strings.EqualFold(asset.Name, want) {
			return asset.BrowserDownloadURL
		}
	}
	for _, asset := range release.Assets {
		if strings.HasSuffix(strings.ToLower(asset.Name), ".zip") && strings.HasPrefix(strings.ToLower(asset.Name), "easycodex-") {
			return asset.BrowserDownloadURL
		}
	}
	return ""
}

func prepareAndStartUpdater(ctx context.Context, installRoot string, info updateCheckResponse) error {
	if strings.TrimSpace(installRoot) == "" {
		return errors.New("install root is empty")
	}
	root, err := filepath.Abs(installRoot)
	if err != nil {
		return err
	}
	staging := filepath.Join(root, ".updates", info.LatestVersion)
	extractRoot := filepath.Join(staging, "extract")
	backupRoot := filepath.Join(root, ".updates", "backup-"+time.Now().Format("20060102-150405"))
	if err := os.RemoveAll(staging); err != nil {
		return err
	}
	if err := os.MkdirAll(extractRoot, 0755); err != nil {
		return err
	}
	zipPath := filepath.Join(staging, "EasyCodex-"+info.LatestVersion+".zip")
	if err := downloadFile(ctx, info.ZipURL, zipPath); err != nil {
		return err
	}
	if err := unzip(zipPath, extractRoot); err != nil {
		return err
	}
	packageRoot, err := locatePackageRoot(extractRoot)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(packageRoot, "EasyCodex.exe")); err != nil {
		return fmt.Errorf("update package is missing EasyCodex.exe: %w", err)
	}
	scriptPath := filepath.Join(staging, "apply-update.ps1")
	logPath := filepath.Join(staging, "apply-update.log")
	if err := os.WriteFile(scriptPath, []byte(updaterScript(root, packageRoot, backupRoot, logPath, os.Getpid())), 0600); err != nil {
		return err
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-File", scriptPath)
	winproc.HideWindow(cmd)
	return cmd.Start()
}

func downloadFile(ctx context.Context, url, dst string) error {
	if strings.TrimSpace(url) == "" {
		return errors.New("download URL is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "EasyCodex-Agent")
	res, err := updateHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("download failed: %s", res.Status)
	}
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, res.Body)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	return os.Rename(tmp, dst)
}

func unzip(zipPath, dst string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, file := range reader.File {
		target := filepath.Join(dst, file.Name)
		rel, err := filepath.Rel(dst, target)
		if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			return fmt.Errorf("unsafe zip entry: %s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, file.Mode()); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		dstFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.Mode())
		if err != nil {
			_ = src.Close()
			return err
		}
		_, copyErr := io.Copy(dstFile, src)
		closeDstErr := dstFile.Close()
		closeSrcErr := src.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeDstErr != nil {
			return closeDstErr
		}
		if closeSrcErr != nil {
			return closeSrcErr
		}
	}
	return nil
}

func locatePackageRoot(extractRoot string) (string, error) {
	if _, err := os.Stat(filepath.Join(extractRoot, "EasyCodex.exe")); err == nil {
		return extractRoot, nil
	}
	entries, err := os.ReadDir(extractRoot)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(extractRoot, entry.Name())
		if _, err := os.Stat(filepath.Join(candidate, "EasyCodex.exe")); err == nil {
			return candidate, nil
		}
	}
	return "", errors.New("update package root was not found")
}

func updaterScript(installRoot, packageRoot, backupRoot, logPath string, pid int) string {
	return fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$installRoot = %s
$packageRoot = %s
$backupRoot = %s
$logPath = %s
$pidToWait = %d
$targetExe = Join-Path $installRoot 'EasyCodex.exe'
$newExe = Join-Path $packageRoot 'EasyCodex.exe'
$targetWez = Join-Path $installRoot 'wezterm-config'
$newWez = Join-Path $packageRoot 'wezterm-config'
$backupExe = Join-Path $backupRoot 'EasyCodex.exe.bak'
$backupWez = Join-Path $backupRoot 'wezterm-config'
function Log($message) {
  $line = "$(Get-Date -Format o) $message"
  Add-Content -LiteralPath $logPath -Value $line
}
try {
  New-Item -ItemType Directory -Force -Path (Split-Path -Parent $logPath) | Out-Null
  New-Item -ItemType Directory -Force -Path $backupRoot | Out-Null
  Log "Waiting for Agent process $pidToWait"
  try { Wait-Process -Id $pidToWait -Timeout 60 -ErrorAction Stop } catch { Log "Wait finished: $($_.Exception.Message)" }
  if (Get-Process -Id $pidToWait -ErrorAction SilentlyContinue) {
    throw "Agent process did not exit within timeout."
  }
  Start-Sleep -Milliseconds 500
  if (-not (Test-Path -LiteralPath $newExe)) { throw "New EasyCodex.exe was not found." }
  if (Test-Path -LiteralPath $targetExe) {
    Copy-Item -LiteralPath $targetExe -Destination $backupExe -Force
  }
  if (Test-Path -LiteralPath $targetWez) {
    Copy-Item -LiteralPath $targetWez -Destination $backupWez -Recurse -Force
  }
  Copy-Item -LiteralPath $newExe -Destination $targetExe -Force
  if (Test-Path -LiteralPath $newWez) {
    if (Test-Path -LiteralPath $targetWez) {
      Remove-Item -LiteralPath $targetWez -Recurse -Force
    }
    Copy-Item -LiteralPath $newWez -Destination $targetWez -Recurse -Force
  }
  Log "Update applied. Starting new Agent."
  Start-Process -FilePath $targetExe -WorkingDirectory $installRoot
} catch {
  Log "ERROR: $($_.Exception.Message)"
  try {
    if (Test-Path -LiteralPath $backupExe) {
      Copy-Item -LiteralPath $backupExe -Destination $targetExe -Force
    }
    if (Test-Path -LiteralPath $backupWez) {
      if (Test-Path -LiteralPath $targetWez) {
        Remove-Item -LiteralPath $targetWez -Recurse -Force
      }
      Copy-Item -LiteralPath $backupWez -Destination $targetWez -Recurse -Force
    }
  } catch {
    Log "RESTORE ERROR: $($_.Exception.Message)"
  }
  exit 1
}
`, psString(installRoot), psString(packageRoot), psString(backupRoot), psString(logPath), pid)
}

func psString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func normalizeVersion(value string) string {
	return strings.TrimPrefix(strings.TrimSpace(strings.ToLower(value)), "v")
}

func isDevVersion(value string) bool {
	v := normalizeVersion(value)
	return v == "" || v == "dev" || strings.Contains(v, "dev")
}

func compareVersions(a, b string) int {
	ap := versionParts(a)
	bp := versionParts(b)
	for i := 0; i < len(ap) || i < len(bp); i++ {
		av, bv := 0, 0
		if i < len(ap) {
			av = ap[i]
		}
		if i < len(bp) {
			bv = bp[i]
		}
		if av > bv {
			return 1
		}
		if av < bv {
			return -1
		}
	}
	return 0
}

var versionPartPattern = regexp.MustCompile(`\d+`)

func versionParts(value string) []int {
	value = normalizeVersion(value)
	value = strings.Split(value, "-")[0]
	matches := versionPartPattern.FindAllString(value, -1)
	parts := make([]int, 0, len(matches))
	for _, match := range matches {
		part, err := strconv.Atoi(match)
		if err == nil {
			parts = append(parts, part)
		}
	}
	return parts
}
