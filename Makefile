.PHONY: test web build check

GO_TAGS := sqlite_fts5

test: web
	go test -tags $(GO_TAGS) ./...
	npm --prefix web test -- --run

web:
	npm --prefix web ci
	npm --prefix web run typecheck
	npm --prefix web run build

build: web
	go build -tags $(GO_TAGS) -o bin/threadhall ./cmd/threadhall
	go build -tags $(GO_TAGS) -o bin/threadhall-agentd ./cmd/threadhall-agentd

check: test build
