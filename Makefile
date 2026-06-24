TARGETS ?= ./cmd/terminus

.PHONY: build clean test
.DEFAULT_GOAL := build
GOBIN ?= $(shell go env GOPATH)/bin

clean:
	go clean
	rm -f ${GOBIN}/terminus

build:
	go install $(TARGETS)

test:
	go test ./... -count=1
	go vet ./...
