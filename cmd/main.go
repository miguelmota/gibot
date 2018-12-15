package main

import (
	"os"

	"github.com/miguelmota/gibot/gibot"
	log "github.com/sirupsen/logrus"
)

func main() {
	accessToken := os.Getenv("GITHUB_ACCESS_TOKEN")
	username := os.Getenv("GITHUB_USERNAME")

	if accessToken == "" {
		log.Fatal("GITHUB_ACCESS_TOKEN is required")
	}
	if username == "" {
		log.Fatal("GITHUB_USERNAME is required")
	}

	bot := gibot.NewBot(&gibot.Config{
		AccessToken: accessToken,
		Username:    username,
	})

	if err := bot.Start(); err != nil {
		log.Error(err)
	}
}
