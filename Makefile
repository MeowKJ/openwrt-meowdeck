.PHONY: check test build run package clean

VERSION ?= dev

check:
	cd web && npm run lint && npm run typecheck
	gofmt -w main.go internal/config/*.go internal/monitor/*.go internal/server/*.go internal/webui/*.go
	go vet . ./internal/...

test:
	cd web && npm test
	go test . ./internal/...

build:
	cd web && npm run build
	mkdir -p bin
	go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/meowdeck .

run: build
	./bin/meowdeck -config configs/config.example.json -listen 127.0.0.1:9080

package:
	VERSION=$(VERSION) GOARCH=arm64 ./scripts/package-release.sh

clean:
	rm -rf bin dist internal/webui/dist/assets internal/webui/dist/index.html

