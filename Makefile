all:
	@echo "no default"

.PHONY: run
run:
	@go run cmd/main.go -queries "ethereum,javascript,blockchain" -username "miguelmota" -search=true -follow=true -unfollow=false -debug=false -store-path="~/.gibot"

.PHONY: build
build:
	@go build -o bin/gibot cmd/main.go
