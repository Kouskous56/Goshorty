.PHONY: help build run test clean install-deps

help:
	@echo "GoShorty - URL Shortener with TTL"
	@echo ""
	@echo "Available commands:"
	@echo "  make install-deps    - Download Go dependencies"
	@echo "  make build          - Build the executable"
	@echo "  make run            - Run the server"
	@echo "  make dev            - Run in development mode (go run)"
	@echo "  make test           - Run tests"
	@echo "  make test-verbose   - Run tests with verbose output"
	@echo "  make clean          - Remove built executable"
	@echo "  make tidy           - Tidy and download dependencies"

install-deps:
	go mod download
	go mod tidy

build: install-deps
	go build -o goshorty.exe -v

run: build
	./goshorty.exe

dev:
	go run main.go

test: install-deps
	go test -v ./...

test-verbose: install-deps
	go test -v -race -coverprofile=coverage.out ./...

clean:
	@if exist goshorty.exe del goshorty.exe
	@echo Cleaned up

tidy: install-deps

.DEFAULT_GOAL := help
