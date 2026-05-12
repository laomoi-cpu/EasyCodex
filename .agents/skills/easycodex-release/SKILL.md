---
name: easycodex-release
description: Release workflow for the EasyCodex repository. Use when publishing a new EasyCodex version, creating/pushing version tags, checking GitHub Actions or GitHub Releases, or updating README release/download links.
---

# EasyCodex Release

## Required Rules

- Treat a version as published only after a GitHub Release exists.
- Do not say "released" when only a tag or Actions artifact exists.
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

5. Monitor GitHub Actions until the tag run completes:

```powershell
Invoke-RestMethod -Uri "https://api.github.com/repos/laomoi-cpu/EasyCodex/actions/runs?per_page=10" -Headers @{ 'User-Agent'='EasyCodex-check' }
```

6. Verify the GitHub Release exists and contains release assets:

```powershell
Invoke-RestMethod -Uri "https://api.github.com/repos/laomoi-cpu/EasyCodex/releases/latest" -Headers @{ 'User-Agent'='EasyCodex-check' }
```

Expected assets:

- `EasyCodex-0.0.x.zip`
- `EasyCodex-0.0.x.patch.zip`
- `EasyCodex.exe`
- `EasyCodex-0.0.x.apk`
- `manifest.json`

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
