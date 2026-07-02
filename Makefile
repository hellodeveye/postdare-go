VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
WEB_DIST := backend/internal/webui/dist
LDFLAGS := -X main.version=$(VERSION)

.PHONY: web build release test

web:
	cd frontend && npm ci && npm run build
	rm -rf $(WEB_DIST)
	mkdir -p $(WEB_DIST)
	cp -R frontend/dist/. $(WEB_DIST)/
	touch $(WEB_DIST)/.gitkeep

build:
	mkdir -p bin
	cd backend && CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o ../bin/postdare-go ./cmd/server

release: web
	mkdir -p bin
	cd backend && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o ../bin/postdare-go-linux-amd64 ./cmd/server
	cd backend && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o ../bin/postdare-go-linux-arm64 ./cmd/server

test:
	cd backend && go vet ./... && go test ./...
