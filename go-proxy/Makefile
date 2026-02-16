BINPATH   = $(PWD)/bin
SHELL     = /bin/bash -euo pipefail

export PATH := $(BINPATH):$(PATH)

.PHONY: tools
tools:
	cd tools && go mod tidy && go mod verify && go generate tools.go

.PHONY: go-deps
go-deps:
	go mod tidy && go mod download && go mod verify

.PHONY: deps
deps: tools go-deps

.PHONY: gen
gen:
	go generate ./...

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: test
test:
	go test -race ./... -coverprofile=coverage.out && go tool cover -func=coverage.out && echo "Ok"

.PHONY: run
run:
	go run cmd/main.go

.PHONY: build
build:
	rm ./dist/proxy && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./dist/proxy ./cmd
