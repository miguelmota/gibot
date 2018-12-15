package gibot

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/google/go-github/github"
	log "github.com/sirupsen/logrus"
	"golang.org/x/oauth2"
)

// Target ...
type target struct {
	username     string
	lastActivity *time.Time
	followed     bool
	followedDate *time.Time
	deleted      bool
}

// Bot ...
type Bot struct {
	client                *github.Client
	username              string
	targets               map[string]*target
	targetFile            string
	originalFollowers     map[string]bool
	originalFollowersFile string
	originalFollowing     map[string]bool
	originalFollowingFile string
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
		client:                client,
		username:              config.Username,
		targets:               make(map[string]*target),
		targetFile:            "targets.csv",
		originalFollowers:     make(map[string]bool),
		originalFollowersFile: "original_followers.csv",
		originalFollowing:     make(map[string]bool),
		originalFollowingFile: "original_following.csv",
	}
}

// Start ...
func (b *Bot) Start() error {
	if _, err := os.Stat(b.originalFollowersFile); !os.IsNotExist(err) {
		f, err := os.Open(b.originalFollowersFile)
		if err != nil {
			return err
		}
		defer f.Close()

		lines, err := csv.NewReader(f).ReadAll()
		if err != nil {
			return err
		}

		for _, line := range lines[1:] {
			b.originalFollowers[line[0]] = true
		}
	} else {
		followers, err := b.getFollowers(b.username)
		if err != nil {
			return err
		}
		if err := b.saveFollowers(followers); err != nil {
			return err
		}
		for _, follower := range followers {
			b.originalFollowers[follower.GetLogin()] = true
		}
	}

	if _, err := os.Stat(b.originalFollowingFile); !os.IsNotExist(err) {
		f, err := os.Open(b.originalFollowingFile)
		if err != nil {
			return err
		}
		defer f.Close()

		lines, err := csv.NewReader(f).ReadAll()
		if err != nil {
			return err
		}

		for _, line := range lines[1:] {
			b.originalFollowing[line[0]] = true
		}
	} else {
		following, err := b.getFollowing(b.username)
		if err != nil {
			return err
		}
		if err := b.saveFollowing(following); err != nil {
			return err
		}
		for _, follower := range following {
			b.originalFollowing[follower.GetLogin()] = true
		}
	}

	if _, err := os.Stat(b.targetFile); !os.IsNotExist(err) {
		f, err := os.Open(b.targetFile)
		if err != nil {
			return err
		}
		defer f.Close()

		lines, err := csv.NewReader(f).ReadAll()
		if err != nil {
			return err
		}

		for _, line := range lines[1:] {
			username := line[0]
			lastActivityStr := line[1]
			followedStr := line[2]
			followedDateStr := line[3]
			var lastActivity *time.Time
			var followedDate *time.Time

			followed, err := strconv.ParseBool(followedStr)
			if err != nil {
				return err
			}

			var deleted bool
			if len(line) > 4 {
				deleted, err = strconv.ParseBool(line[4])
				if err != nil {
					return err
				}
			}

			if lastActivityStr != "" {
				i, err := strconv.ParseInt(lastActivityStr, 10, 64)
				if err != nil {
					return err
				}
				t := time.Unix(i, 0)
				lastActivity = &t
			}

			if followedDateStr != "" {
				i, err := strconv.ParseInt(followedDateStr, 10, 64)
				if err != nil {
					return err
				}
				t := time.Unix(i, 0)
				followedDate = &t
			}

			b.targets[username] = &target{
				username:     username,
				lastActivity: lastActivity,
				followed:     followed,
				followedDate: followedDate,
				deleted:      deleted,
			}
		}
	}

	search := false
	if search {
		query := "ethereum"
		users, err := b.searchUsers(query, 100)
		if err != nil {
			return err
		}

		var wg sync.WaitGroup
		for _, user := range users {
			wg.Add(1)
			go func(user github.User) {
				defer wg.Done()
				time.Sleep(1 * time.Second)
				username := user.GetLogin()
				isActive, lastActivity, err := b.isActive(username)
				if err != nil {
					log.Errorf("got error; %s\n", err)
					return
				}
				if isActive {
					_, found := b.targets[username]
					if !found {
						b.targets[username] = &target{
							username:     username,
							lastActivity: lastActivity,
							followed:     false,
							followedDate: nil,
							deleted:      false,
						}
					}
				}
			}(user)
		}
		wg.Wait()

		log.Printf("targets found %v\n", len(b.targets))

		if err := b.saveTargets(); err != nil {
			return err
		}
	}

	go b.handleExitSignal()
	if err := b.startFollowing(); err != nil {
		return err
	}
	if err := b.saveTargets(); err != nil {
		return err
	}

	/*
		if err := b.deleteFollowing(); err != nil {
			return err
		}
	*/

	return nil
}

func (b *Bot) handleExitSignal() {
	var gracefulStop = make(chan os.Signal)
	signal.Notify(gracefulStop, syscall.SIGTERM)
	signal.Notify(gracefulStop, syscall.SIGINT)
	go func() {
		sig := <-gracefulStop
		log.Printf("caught signal: %+v\n", sig)
		log.Println("saving latest state...")
		if err := b.saveTargets(); err != nil {
			panic(err)
		}
		os.Exit(0)
	}()
}

func (b *Bot) deleteFollowing() error {
	for _, target := range b.targets {
		if !target.followed || target.deleted {
			continue
		}
		time.Sleep(5 * time.Second)
		if err := b.unfollow(target.username); err != nil {
			log.Errorf("unfollow error: %v", err)
			continue
		}
		log.Printf("unfollowed following: %s\n", target.username)
		b.targets[target.username].deleted = true
	}

	return nil
}

func (b *Bot) startFollowing() error {
	for _, target := range b.targets {
		if target.followed {
			continue
		}
		time.Sleep(5 * time.Second)
		if err := b.follow(target.username); err != nil {
			log.Errorf("follow error: %v", err)
			continue
		}
		log.Printf("followed target: %s\n", target.username)
		b.targets[target.username].followed = true
		t := time.Now()
		b.targets[target.username].followedDate = &t
	}

	return nil
}

func (b *Bot) saveTargets() error {
	records := [][]string{
		[]string{"username", "last_activity", "followed", "followed_date", "deleted"},
	}
	for _, target := range b.targets {
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
			fmt.Sprintf("%v", target.deleted),
		})
	}

	fo, err := os.Create(b.targetFile)
	if err != nil {
		return err
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
		return err
	}

	log.Println("done")
	return nil
}

func (b *Bot) saveFollowers(followers []*github.User) error {
	records := [][]string{
		[]string{"username"},
	}
	for _, follower := range followers {
		records = append(records, []string{
			follower.GetLogin(),
		})
	}

	fo, err := os.Create(b.originalFollowersFile)
	if err != nil {
		return err
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
		return err
	}

	return nil
}

func (b *Bot) saveFollowing(following []*github.User) error {
	records := [][]string{
		[]string{"username"},
	}
	for _, follower := range following {
		records = append(records, []string{
			follower.GetLogin(),
		})
	}

	fo, err := os.Create(b.originalFollowingFile)
	if err != nil {
		return err
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
		return err
	}

	return nil
}

func (b *Bot) isActive(username string) (bool, *time.Time, error) {
	events, resp, err := b.client.Activity.ListEventsPerformedByUser(context.Background(), username, false, &github.ListOptions{
		Page:    0,
		PerPage: 2,
	})
	if int(resp.StatusCode/100) != 2 {
		log.Errorf("received status code %v\n", resp.StatusCode)
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
		log.Errorf("received status code %v\n", resp.StatusCode)
		return false, errors.New(resp.Status)
	}
	if err != nil {
		return false, err
	}

	return isFollowing, nil
}

func (b *Bot) follow(username string) error {
	resp, err := b.client.Users.Follow(context.Background(), username)
	if int(resp.StatusCode/100) != 2 {
		log.Errorf("received status code %v\n", resp.StatusCode)
		return errors.New(resp.Status)
	}
	if err != nil {
		return err
	}

	return nil
}

func (b *Bot) unfollow(username string) error {
	resp, err := b.client.Users.Unfollow(context.Background(), username)
	if int(resp.StatusCode/100) != 2 {
		log.Errorf("received status code %v\n", resp.StatusCode)
		return errors.New(resp.Status)
	}
	if err != nil {
		return err
	}

	return nil
}

func (b *Bot) searchUsers(query string, limit int) ([]github.User, error) {
	var collection []github.User
	page := 1
	lastSize := 100
	maxPage := 5
	for i := 0; lastSize >= 100 && page < maxPage; i++ {
		result, resp, err := b.client.Search.Users(context.Background(), query, &github.SearchOptions{
			ListOptions: github.ListOptions{
				Page:    page,
				PerPage: limit,
			},
		})
		if err != nil {
			return []github.User{}, err
		}
		if resp.StatusCode != 200 {
			log.Errorf("received status code %v\n", resp.StatusCode)
			break
		}
		lastSize = len(result.Users)
		page++
		log.Printf("fetching %v followers\n", lastSize)
		collection = append(collection, result.Users...)
	}

	return collection, nil
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
			log.Errorf("received status code %v\n", resp.StatusCode)
			break
		}
		lastSize = len(followers)
		page++
		log.Printf("fetching %v followers\n", lastSize)
		collection = append(collection, followers...)
	}

	log.Printf("fetched %v followers\n", len(collection))

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
			log.Errorf("received status code %v\n", resp.StatusCode)
			break
		}
		lastSize = len(following)
		page++
		log.Printf("fetching %v following\n", lastSize)
		collection = append(collection, following...)
	}

	log.Printf("fetched %v following\n", len(collection))

	return collection, nil
}

func init() {
	log.SetReportCaller(true)
}
