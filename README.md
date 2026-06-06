# dataseai-connector

LAN agent for the dataseai cloud service. Runs on a machine inside your
network, connects out to dataseai.app over WebSocket, and proxies SQL
queries to your private MySQL servers — no inbound ports required.

Status: **WIP** — handshake + heartbeat works end-to-end against the
in-repo test broker; no MySQL relay yet.

## How it works

```
[your browser]
     ↓ HTTPS
[dataseai.app]                ← cloud service
     ↓ WebSocket (you receive on the open connection)
[dataseai-connector]          ← this program, in your LAN
     ↓ TCP
[your MySQL]
```

The connector authenticates with a per-user token, stays connected over
WebSocket, and runs each incoming SQL request against the configured local
MySQL servers. Credentials never leave your machine.

## Layout

```
cmd/connector/        the agent that ships to users
cmd/test-broker/      throwaway local broker for development
pkg/protocol/         JSON message types shared by both sides
```

## Build from source

```
git clone https://github.com/cin-yi-wei/dataseai-connector
cd dataseai-connector
go build ./...
```

## Run the end-to-end demo

In one terminal:

```
go run ./cmd/test-broker
```

In another:

```
go run ./cmd/connector --token=ag_test_anything --server=ws://localhost:8080/agent
```

You should see the connector authenticate, exchange ping/pong every 5s, and
reconnect automatically if the broker goes away.

## Install

Coming soon. Planned channels:

- Linux `.deb` / `.rpm`
- macOS `.pkg` and Homebrew tap
- Windows `.msi` and `winget`
- Single binary for every platform
- Docker image

## Roadmap

- [x] Cross-platform build pipeline (GoReleaser, 6 targets)
- [x] WebSocket client + reconnect with backoff
- [x] Hello / HelloAck / Heartbeat protocol
- [x] Throwaway test broker for local development
- [ ] Query relay (MySQL execution + streaming results back)
- [ ] System service install (kardianos/service, all platforms)
- [ ] HTTPS_PROXY support
- [ ] Auto-update mechanism
- [ ] Code signing (Apple Developer ID, Windows EV)

## License

MIT
