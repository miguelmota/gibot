all:
	@echo "no default"

.PHONY: run
run:
	@go run cmd/main.go -queries "ethereum,blockchain" -username "miguelmota" -search=false -follow=false -unfollow=true -debug=false -store-path="~/.gibot"

.PHONY: build
build:
	@go build -o bin/gibot cmd/main.go
