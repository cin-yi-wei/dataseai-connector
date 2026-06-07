# Windows GUI Preview

This preview is unsigned. Windows SmartScreen may warn on first run. Right-click → "Run anyway" to proceed.

## Install

1. Download `dataseai-connector-gui.exe` and `dataseai-connector.exe` to the same folder.
2. Run `dataseai-connector-gui.exe` as Administrator.
3. Paste the agent token from the DataseAI Local Agents page.
4. Click **Install & Start**.

## What It Does

- Writes config to `%ProgramData%\dataseai-connector\config.yaml`
- Installs and starts the Windows service `dataseai-connector`
- Shows service status and recent logs

## Troubleshooting

- **Install fails:** Run as Administrator.
- **Service running but agent offline:** Check server URL and network.
- **Need help:** Click **Copy Diagnostics** before reporting a bug.
