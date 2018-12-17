package main

import (
	"flag"
	"os"
	"strings"

	"github.com/miguelmota/gibot/gibot"
	log "github.com/sirupsen/logrus"
)

func main() {
	accessToken := os.Getenv("GITHUB_ACCESS_TOKEN")

	search := flag.Bool("search", false, "Search")
	queries := flag.String("queries", "", "Queries")
	follow := flag.Bool("follow", false, "Follow")
	unfollow := flag.Bool("unfollow", false, "Unfollow")
	username := flag.String("username", os.Getenv("GITHUB_USERNAME"), "username")
	storePath := flag.String("store-path", "", "Store path")
	debug := flag.Bool("debug", false, "Debug")
	flag.Parse()

	if *debug {
		log.SetReportCaller(true)
	}

	if accessToken == "" {
		log.Fatal("GITHUB_ACCESS_TOKEN is required")
	}
	if *username == "" {
		log.Fatal("username is required")
	}

	bot := gibot.NewBot(&gibot.Config{
		AccessToken: accessToken,
		Username:    *username,
		StorePath:   *storePath,
	})

	searchQueries := strings.Split(*queries, ",")

	log.Printf("config search: %v\n", *search)
	log.Printf("config queries: %v\n", searchQueries)
	log.Printf("config follow: %v\n", *follow)
	log.Printf("config unfollow: %v\n", *unfollow)
	log.Printf("config store path: %s\n", *storePath)

	if err := bot.Start(&gibot.StartConfig{
		Search:   *search,
		Queries:  searchQueries,
		Follow:   *follow,
		Unfollow: *unfollow,
	}); err != nil {
		log.Error(err)
	}
}
