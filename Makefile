# dataseai-connector dev shortcuts
#
#   make build         — build both binaries to ./dist-local
#   make run-mock      — run broker + connector locally (mock data)
#   make snapshot      — local goreleaser snapshot (no upload)
#   make ci            — what GitHub Actions ci.yml runs locally
#   make push          — git push current branch
#   make release v=X.Y.Z
#                      — fast-forward main to dev's HEAD, tag vX.Y.Z,
#                        push tag (triggers GH Actions release)

PROJECT_DIR := $(CURDIR)
BUILD_DIR   := $(PROJECT_DIR)/dist-local

.PHONY: build run-mock snapshot ci push release clean gui-build gui-package-windows

build:
	mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/dataseai-connector ./cmd/connector
	go build -o $(BUILD_DIR)/test-broker ./cmd/test-broker

run-mock: build
	@echo "starting broker on :8080 and connector with mock executor"
	@trap 'kill 0' EXIT INT TERM; \
		$(BUILD_DIR)/test-broker & sleep 1; \
		$(BUILD_DIR)/dataseai-connector --executor=mock --token=devtoken

snapshot:
	rm -rf dist
	goreleaser release --snapshot --clean --skip=publish
	@echo "→ artifacts in ./dist/"

ci:
	go vet ./...
	go test ./... -count=1
	go build ./...
	goreleaser check

push:
	@git -C $(PROJECT_DIR) status --short
	git -C $(PROJECT_DIR) push origin HEAD

release:
	@test -n "$(v)" || (echo "usage: make release v=X.Y.Z" && exit 1)
	@cd $(PROJECT_DIR) && \
		[ -z "$$(git status --porcelain)" ] || (echo "working tree dirty, commit first" && exit 1)
	@cd $(PROJECT_DIR) && git fetch origin
	@echo "→ fast-forwarding remote main to dev's HEAD"
	cd $(PROJECT_DIR) && git push origin dev:main
	cd $(PROJECT_DIR) && git branch -f main origin/main
	@echo "→ tagging v$(v) on main"
	cd $(PROJECT_DIR) && git tag -a "v$(v)" main -m "release v$(v)"
	cd $(PROJECT_DIR) && git push origin "v$(v)"
	@echo "→ GH Actions will build + publish in ~3 min"
	@echo "watch: https://github.com/cin-yi-wei/dataseai-connector/actions"

gui-build:
	cd cmd/connector-gui && wails build

gui-package-windows:
	GOOS=windows GOARCH=amd64 go build -o $(BUILD_DIR)/dataseai-connector.exe ./cmd/connector
	cd cmd/connector-gui && wails build -platform windows/amd64

clean:
	rm -rf $(BUILD_DIR) dist
