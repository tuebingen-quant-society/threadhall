.PHONY: test web build check

test: web
	go test ./...
	npm --prefix web test -- --run

web:
	npm --prefix web ci
	npm --prefix web run build

build: web
	go build -o bin/threadhall ./cmd/threadhall

check: test build
