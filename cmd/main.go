package main

import (
	"encoding/csv"
	"flag"
	"os"
	"strings"

	"github.com/miguelmota/gibot/gibot"
	log "github.com/sirupsen/logrus"
)

func main() {
	accessToken := os.Getenv("GITHUB_ACCESS_TOKEN")

	cmd := os.Args[len(os.Args)-1 : len(os.Args)][0]
	search := flag.Bool("search", false, "Search")
	queries := flag.String("queries", "", "Queries")
	follow := flag.Bool("follow", false, "Follow")
	unfollow := flag.Bool("unfollow", false, "Unfollow")
	username := flag.String("username", os.Getenv("GITHUB_USERNAME"), "username")
	storePath := flag.String("store-path", "", "Store path")
	file := flag.String("file", "", "Filepath")
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

	if cmd == "unfollow" {
		log.Println("starting unfollowing all targets")
		file := gibot.NormalizePath(*file)

		f, err := os.Open(file)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()

		lines, err := csv.NewReader(f).ReadAll()
		if err != nil {
			log.Fatal(err)
		}

		var targets []string
		for _, line := range lines[1:] {
			targets = append(targets, line[0])
		}

		for _, target := range targets {
			if err := bot.Unfollow(target); err != nil {
				log.Errorf("unfollow target error: %v", err)
				continue
			}
			log.Printf("unfollowed target %q\n", target)
			bot.ThrottleWait()
		}

		log.Println("done unfollowing all followed targets")
	} else {
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
}
