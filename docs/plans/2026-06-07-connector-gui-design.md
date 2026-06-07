# DataseAI Connector GUI Design

Date: 2026-06-07

## Goal

Make `dataseai-connector` usable by non-CLI users. The first release should
feel like a normal Windows desktop utility: paste an agent token, install and
start the connector, see whether it is online, and copy diagnostics when
something fails.

The GUI does not replace the connector core. It wraps the existing connector
binary and service lifecycle.

## Scope

First release:

- Windows desktop GUI first.
- Cross-platform structure, but macOS and Linux are not first-class release
  targets yet.
- Manage the existing connector service.
- Write and read the existing config file:
  `%ProgramData%\dataseai-connector\config.yaml`.
- Show both service status and DataseAI agent status.
- Show recent logs.
- Provide copyable diagnostics.
- Run in the system tray.

Out of scope for the first release:

- macOS notarization, Windows code signing, or polished Linux packaging.
- Auto-update.
- Editing DB targets locally.
- Moving database credentials out of DataseAI web connections.
- Multi-agent management on one machine.

## Product Shape

The GUI should expose the smallest useful workflow:

1. User creates an agent in DataseAI web and copies the one-time token.
2. User opens the connector GUI.
3. User pastes token and keeps the default server URL.
4. User clicks `Install & Start`.
5. GUI writes config, installs the Windows service, starts it, and checks
   DataseAI agent status.
6. GUI stays available from the system tray for status, restart, logs, and
   diagnostics.

The GUI should not ask users to understand WebSocket URLs, Windows Services,
or terminal commands unless troubleshooting requires it.

## Technical Approach

Use Wails for the desktop shell.

Reasons:

- Go backend matches the existing connector.
- React/Vite frontend matches the DataseAI web stack.
- Smaller than Electron.
- Can later extend to macOS and Linux without changing the whole architecture.

The repository should keep the connector core independent from GUI code:

```text
cmd/connector/        existing CLI and service binary
cmd/connector-gui/    Wails desktop app
internal/control/     service/config/log/status helpers shared by CLI and GUI
pkg/protocol/         existing broker protocol
```

The exact package split can change during implementation, but service/config
logic should not be duplicated between CLI and GUI.

## Configuration

The GUI writes the same config format the service already uses:

```yaml
token: ag_xxxxx
server: wss://dataseai.conray.top/agent
executor: mysql
```

Default server URL for this project:

```text
wss://dataseai.conray.top/agent
```

The token is sensitive. The config file should keep owner-only permissions
where the platform supports it. The GUI should not echo the full token after
save; show a masked token summary instead.

## Status Model

Show two separate statuses because they fail for different reasons:

- Service status: `not installed`, `stopped`, `running`, `unknown`.
- Agent status: `online`, `offline`, `connecting`, `auth failed`, `unknown`.

Service status comes from local service control.

Agent status should come from DataseAI when possible. If the GUI cannot call
DataseAI yet, first release can show agent status from local connector logs,
but the UI should be designed so a server-side status API can replace it.

## Logs And Diagnostics

The GUI should show recent connector logs and provide a copy diagnostics
button. Diagnostics should include:

- Connector version, commit, build date.
- OS and architecture.
- Config path.
- Server URL.
- Executor.
- Service status.
- Agent status if available.
- Last relevant log lines.

Diagnostics must not include the full token.

The current connector may need small CLI improvements:

- `status --json` for machine-readable status.
- A stable log location or `logs` command.
- Config validation command.
- Less stdout-only behavior for install/start errors.

## UI

Primary screen:

- Status header with service and agent state.
- Token input with paste support.
- Server URL field, advanced by default.
- Buttons: `Install & Start`, `Restart`, `Stop`, `Copy Diagnostics`.
- Recent logs panel.

System tray:

- Show current status.
- Open window.
- Restart connector.
- Stop connector.
- Quit GUI.

The GUI should be quiet and operational, not marketing-styled. It is a small
utility for repeated troubleshooting.

## Error Handling

Common cases should be translated into useful messages:

- Missing token: ask user to create/copy a token from DataseAI.
- Service install permission denied: ask user to run installer/GUI as admin.
- Service running but agent offline: show WebSocket/auth/network guidance.
- Auth failed: token is invalid or revoked; create a new agent token.
- Broker unreachable: check server URL, network, proxy, or firewall.

Raw error details should remain visible in diagnostics/logs.

## Testing

Unit tests:

- Config read/write and token masking.
- Status parsing.
- Diagnostic payload redaction.
- Service adapter behavior with fake adapter.

Integration/manual tests:

- Fresh Windows install.
- Existing config update.
- Start/stop/restart service.
- Invalid token.
- Broker unreachable.
- Existing connector CLI still works.

## Release Strategy

First release is unsigned Windows preview.

It can ship as:

- `dataseai-connector-gui.exe` plus `dataseai-connector.exe`, or
- an installer that bundles both.

Windows SmartScreen warnings are acceptable for preview. Code signing can be
handled later if the tool gets external users.
