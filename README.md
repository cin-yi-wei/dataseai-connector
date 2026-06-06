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

### Mock data (no MySQL needed)

In one terminal:

```
go run ./cmd/test-broker
```

In another:

```
go run ./cmd/connector --executor=mock --token=anything
```

You should see the connector authenticate, then 2s later receive a query,
stream 5 batches of 10 fake rows back, plus ping/pong every 5s.

### Against a real MySQL

Point the broker's hardcoded demo query at a real server via env:

```
TARGET_HOST=192.168.50.250 \
TARGET_PORT=3306 \
TARGET_USER=root \
TARGET_PASS=secret \
TARGET_DB=mysql \
TARGET_SQL="SELECT user, host FROM user LIMIT 5" \
go run ./cmd/test-broker
```

And run the connector with `--executor=mysql` (the default):

```
go run ./cmd/connector --token=anything
```

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
- [x] Query relay protocol (request → meta → rows × N → done | error)
- [x] Mock executor (50 fake rows, 5 batches)
- [x] Real MySQL executor (`database/sql` + `go-sql-driver/mysql`)
- [ ] Multi-target connector config (which DBs am I allowed to reach?)
- [ ] System service install (kardianos/service, all platforms)
- [ ] HTTPS_PROXY support
- [ ] Auto-update mechanism
- [ ] Code signing (Apple Developer ID, Windows EV)

## License

MIT
