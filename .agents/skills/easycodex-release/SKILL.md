---
name: easycodex-release
description: Release workflow for the EasyCodex repository. Use when publishing a new EasyCodex version, creating/pushing version tags, checking GitHub Actions or GitHub Releases, or updating README release/download links.
---

# EasyCodex Release

## Required Rules

- Treat a version as published only after a GitHub Release exists.
- Do not say "released" when only a tag or Actions artifact exists.
- Do not rely only on the GitHub Actions web page status. It can lag or show a cached `In progress` state after the Release is already published.
- Every release must update the top of `README.md` with the latest built zip download link.
- Do not commit local runtime files such as `agent/config.json` or `task.md`.
- Use Chinese Git commit messages for project commits.

## Release Checklist

1. Confirm working tree and ignore local-only files:

```powershell
git status --short --branch
```

2. Update the top of `README.md` with the latest zip link for the version being released:

```markdown
最新版本下载：[EasyCodex 0.0.x](https://github.com/laomoi-cpu/EasyCodex/releases/download/v0.0.x/EasyCodex-0.0.x.zip)
```

Keep the link near the top, before the product introduction. Use the zip as the primary download, not only the exe or apk.

3. Commit the README change and any release workflow changes:

```powershell
git add README.md .github/workflows/release.yml
git commit -m "更新README最新下载链接"
```

4. Create and push the version tag:

```powershell
git tag v0.0.x
git push origin master
git push origin v0.0.x
```

If HTTPS GitHub push times out but SSH works, use:

```powershell
git push git@github.com:laomoi-cpu/EasyCodex.git master
git push git@github.com:laomoi-cpu/EasyCodex.git v0.0.x
```

5. Monitor GitHub Actions until the tag run completes when the API is available:

```powershell
Invoke-RestMethod -Uri "https://api.github.com/repos/laomoi-cpu/EasyCodex/actions/runs?per_page=10" -Headers @{ 'User-Agent'='EasyCodex-check' }
```

If the GitHub REST API is rate-limited or the Actions web page appears stale, switch to Release verification instead of waiting on the cached Actions page.

6. Verify the GitHub Release exists and contains release assets. Prefer the API when available:

```powershell
Invoke-RestMethod -Uri "https://api.github.com/repos/laomoi-cpu/EasyCodex/releases/latest" -Headers @{ 'User-Agent'='EasyCodex-check' }
```

Expected assets:

- `EasyCodex-0.0.x.zip`
- `EasyCodex-0.0.x.patch.zip`
- `EasyCodex.exe`
- `EasyCodex-0.0.x.apk`
- `manifest.json`

When API verification is unavailable, verify the release page and asset URLs directly:

```powershell
$version = "0.0.x"
$tag = "v$version"
$assets = @(
  "EasyCodex-$version.zip",
  "EasyCodex-$version.patch.zip",
  "EasyCodex.exe",
  "EasyCodex-$version.apk",
  "manifest.json"
)
foreach ($asset in $assets) {
  curl.exe -I --max-time 20 -L "https://github.com/laomoi-cpu/EasyCodex/releases/download/$tag/$asset"
}
```

For direct asset checks, either a GitHub `302 Found` redirect to `release-assets.githubusercontent.com` or a final `200 OK` confirms the asset exists. A `404 Not Found` means the asset is not published yet. Large assets may time out after the redirect on slow networks; if the redirect is present and names the expected `filename=...`, treat the asset as present and cross-check the Release page.

## Workflow Requirement

`.github/workflows/release.yml` must create a GitHub Release on tag builds, not only upload an artifact. The workflow should:

- run on tags matching `v*.*.*`
- have `permissions: contents: write`
- build with `scripts/build-release.ps1`
- create `EasyCodex-<version>.zip`
- create `EasyCodex-<version>.patch.zip` containing only files used by auto-update (`EasyCodex.exe` and `wezterm-config/`)
- upload the artifact
- publish a GitHub Release with full zip, patch zip, fixed-name `EasyCodex.exe`, apk, and manifest assets

## README Download Link

When publishing `v0.0.x`, the README top link must point to:

```text
https://github.com/laomoi-cpu/EasyCodex/releases/download/v0.0.x/EasyCodex-0.0.x.zip
```

After release completes, verify this URL is available from the GitHub Release assets.
