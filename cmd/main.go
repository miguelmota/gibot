package main

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

// Bot ...
type Bot struct {
	client   *github.Client
	username string
}

// Config ...
type Config struct {
	AccessToken string
	Username    string
}

// NewBot ...
func NewBot(config *Config) *Bot {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{
			AccessToken: config.AccessToken,
		},
	)
	tc := oauth2.NewClient(context.Background(), ts)
	client := github.NewClient(tc)
	return &Bot{
		client:   client,
		username: config.Username,
	}
}

// Target ...
type target struct {
	username     string
	lastActivity *time.Time
	followed     bool
	followedDate *time.Time
}

func main() {
	accessToken := os.Getenv("GITHUB_ACCESS_TOKEN")
	username := os.Getenv("GITHUB_USERNAME")

	if accessToken == "" {
		panic("GITHUB_ACCESS_TOKEN is required")
	}
	if username == "" {
		panic("GITHUB_USERNAME is required")
	}

	bot := NewBot(&Config{
		AccessToken: accessToken,
		Username:    username,
	})

	targets := make(map[string]*target)
	query := "ethereum"
	users, err := bot.searchUsers(query, 100)
	if err != nil {
		panic(err)
	}

	var wg sync.WaitGroup
	for _, user := range users {
		wg.Add(1)
		go func(user github.User) {
			defer wg.Done()
			time.Sleep(1 * time.Second)
			username := user.GetLogin()
			isActive, lastActivity, err := bot.isActive(username)
			if err != nil {
				fmt.Printf("got error; %s\n", err)
				return
			}
			if isActive {
				_, found := targets[username]
				if !found {
					targets[username] = &target{
						username:     username,
						lastActivity: lastActivity,
						followed:     false,
						followedDate: nil,
					}
				}
			}
		}(user)
	}
	wg.Wait()

	fmt.Printf("targets found %v\n", len(targets))

	records := [][]string{
		[]string{"username", "last_activity", "followed", "followed_date"},
	}
	for _, target := range targets {
		var lastActivity int64
		if target.lastActivity != nil {
			lastActivity = target.lastActivity.Unix()
		}
		var followedDate int64
		if target.followedDate != nil {
			followedDate = target.followedDate.Unix()
		}
		records = append(records, []string{
			target.username,
			fmt.Sprintf("%v", lastActivity),
			fmt.Sprintf("%v", target.followed),
			fmt.Sprintf("%v", followedDate),
		})
	}

	fo, err := os.Create("targets.csv")
	if err != nil {
		panic(err)
	}
	w := csv.NewWriter(fo)

	for _, record := range records {
		if err := w.Write(record); err != nil {
			log.Fatalln("error writing record to csv:", err)
		}
	}

	// Write any buffered data to the underlying writer (standard output).
	w.Flush()

	if err := w.Error(); err != nil {
		panic(err)
	}

	fmt.Println("done")
}

func (b *Bot) isActive(username string) (bool, *time.Time, error) {
	events, resp, err := b.client.Activity.ListEventsPerformedByUser(context.Background(), username, false, &github.ListOptions{
		Page:    0,
		PerPage: 2,
	})
	if int(resp.StatusCode/100) != 2 {
		fmt.Printf("received status code %v\n", resp.StatusCode)
		return false, nil, errors.New(resp.Status)
	}
	if err != nil {
		return false, nil, err
	}
	if len(events) >= 2 {
		recent := time.Now().Add(-time.Duration(48 * time.Hour))
		if events[0].CreatedAt.After(recent) && events[1].CreatedAt.After(recent) {
			return true, events[0].CreatedAt, nil
		}
	}

	return false, nil, nil
}

func (b *Bot) isFollowing(username string) (bool, error) {
	isFollowing, resp, err := b.client.Users.IsFollowing(context.Background(), b.username, username)
	if int(resp.StatusCode/100) != 2 {
		fmt.Printf("received status code %v\n", resp.StatusCode)
		return false, errors.New(resp.Status)
	}
	if err != nil {
		return false, err
	}
	fmt.Printf("is following %s: %v\n", username, isFollowing)
	return isFollowing, nil
}

func (b *Bot) follow(username string) error {
	resp, err := b.client.Users.Follow(context.Background(), username)
	if int(resp.StatusCode/100) != 2 {
		fmt.Printf("received status code %v\n", resp.StatusCode)
		return errors.New(resp.Status)
	}
	if err != nil {
		return err
	}
	fmt.Printf("followed %s\n", username)

	return nil
}

func (b *Bot) searchUsers(query string, limit int) ([]github.User, error) {
	result, resp, err := b.client.Search.Users(context.Background(), query, &github.SearchOptions{
		ListOptions: github.ListOptions{
			Page:    0,
			PerPage: limit,
		},
	})
	if err != nil {
		return []github.User{}, err
	}
	if resp.StatusCode != 200 {
		fmt.Printf("received status code %v\n", resp.StatusCode)
		return []github.User{}, errors.New(resp.Status)
	}

	return result.Users, nil
}

func (b *Bot) getFollowers(username string) ([]*github.User, error) {
	var collection []*github.User
	page := 1
	lastSize := 100
	for i := 0; lastSize >= 100; i++ {
		followers, resp, err := b.client.Users.ListFollowers(context.Background(), username, &github.ListOptions{
			Page:    page,
			PerPage: 100,
		})
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != 200 {
			fmt.Printf("received status code %v\n", resp.StatusCode)
			break
		}
		lastSize = len(followers)
		page++
		fmt.Printf("fetching %v followers\n", lastSize)
		collection = append(collection, followers...)
	}

	fmt.Printf("fetched %v followers\n", len(collection))

	return collection, nil
}

func (b *Bot) getFollowing(username string) ([]*github.User, error) {
	var collection []*github.User
	page := 1
	lastSize := 100
	for i := 0; lastSize >= 100; i++ {
		following, resp, err := b.client.Users.ListFollowing(context.Background(), username, &github.ListOptions{
			Page:    page,
			PerPage: 100,
		})
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != 200 {
			fmt.Printf("received status code %v\n", resp.StatusCode)
			break
		}
		lastSize = len(following)
		page++
		fmt.Printf("fetching %v following\n", lastSize)
		collection = append(collection, following...)
	}

	fmt.Printf("fetched %v following\n", len(collection))

	return collection, nil
}
