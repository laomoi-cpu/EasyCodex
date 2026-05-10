# EasyCodex Android MVP

Lightweight Android controller for EasyCodex Agent.

## Build

This project targets the Unity-installed Android SDK in the current workstation:

```powershell
$env:JAVA_HOME = "C:\Program Files\Unity\Hub\Editor\2021.3.12f1\Editor\Data\PlaybackEngines\AndroidPlayer\OpenJDK"
$env:ANDROID_HOME = "C:\Program Files\Unity\Hub\Editor\2021.3.12f1\Editor\Data\PlaybackEngines\AndroidPlayer\SDK"
```

Use Gradle from `android\.gradle-local` or any compatible Gradle 6.x install:

```powershell
.\.gradle-local\gradle-6.7.1\bin\gradle.bat assembleDebug
```

Debug APK output:

```text
app\build\outputs\apk\debug\app-debug.apk
```

## Agent Setup

For phone access, set Agent listen address to:

```json
{
  "listen": "0.0.0.0:8765"
}
```

Then connect the APK to `http://<pc-lan-ip>:8765` with the configured token.

## MVP Features

- Test and save Agent connection settings.
- Verify `/api/health` and `/api/instances`.
- Load `main` sessions and show pane buttons.
- Poll pane snapshots every second using the server hash.
- Send UTF-8 text as base64 with optional Enter.
- Shortcut buttons: Enter, Ctrl+C, Tab, Esc.
