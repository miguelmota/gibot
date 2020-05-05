all:
	@echo "no default"

.PHONY: run
run:
	@go run cmd/main.go -queries="ethereum,blockchain" -username="miguelmota" -search=false -follow=false -unfollow=true -debug=false -store-path="~/.gibot2"

.PHONY: unfollow
unfollow:
	@go run cmd/main.go -file="unfollow.csv" -username="miguelmota" unfollow

.PHONY: diff-unfollow
diff-unfollow:
	@diff -c original_following.csv.bak ~/.gibot/original_following.csv | grep '+' | awk '{$1=$2=""; print $0}' > unfollow.csv

.PHONY: build
build:
	@go build -o bin/gibot cmd/main.go
