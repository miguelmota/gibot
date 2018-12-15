all:
	@echo "no default"

.PHONY: run
run:
	@go run cmd/main.go

.PHONY: build
build:
	@go build -o bin/gibot cmd/main.go
