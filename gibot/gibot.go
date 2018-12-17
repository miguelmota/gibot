package gibot

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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
	StorePath   string
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

	configPath := normalizePath(config.StorePath)

	if configPath == "" {
		configPath = "./"
	}

	targetFile := fmt.Sprintf("%s/targets.csv", configPath)
	followersFile := fmt.Sprintf("%s/original_followers.csv", configPath)
	followingFile := fmt.Sprintf("%s/original_following.csv", configPath)
	return &Bot{
		client:                client,
		username:              config.Username,
		targets:               make(map[string]*target),
		targetFile:            targetFile,
		originalFollowers:     make(map[string]bool),
		originalFollowersFile: followersFile,
		originalFollowing:     make(map[string]bool),
		originalFollowingFile: followingFile,
	}
}

// StartConfig ...
type StartConfig struct {
	Search   bool
	Queries  []string
	Follow   bool
	Unfollow bool
}

// Start ...
func (b *Bot) Start(config *StartConfig) error {
	go b.handleExitSignal()

	search := config.Search
	queries := config.Queries
	followTargets := config.Follow
	unfollowTargets := config.Unfollow

	err := b.loadState()
	if err != nil {
		return err
	}

	if search {
		err := b.searchActiveUsers(queries)
		if err != nil {
			return err
		}
		if err := b.saveTargets(); err != nil {
			return err
		}
	}

	if followTargets {
		if err := b.followTargets(); err != nil {
			return err
		}
		if err := b.saveTargets(); err != nil {
			return err
		}
	}

	if unfollowTargets {
		if err := b.unfollowTargets(); err != nil {
			return err
		}
		if err := b.saveTargets(); err != nil {
			return err
		}
	}

	return nil
}

func (b *Bot) loadState() error {
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
	return nil
}

func (b *Bot) followTargets() error {
	log.Println("starting following of targets")
	for _, target := range b.targets {
		if target.followed {
			continue
		}
		if err := b.follow(target.username); err != nil {
			log.Errorf("follow target error: %v", err)
			continue
		}
		log.Printf("followed target user %q\n", target.username)
		b.targets[target.username].followed = true
		t := time.Now()
		b.targets[target.username].followedDate = &t
		randomSleep()
	}

	log.Println("done following all targets")
	return nil
}

func (b *Bot) unfollowTargets() error {
	log.Println("starting unfollowing all targets")
	for _, target := range b.targets {
		_, ok := b.originalFollowing[target.username]
		if ok || target.deleted || !target.followed {
			continue
		}
		if err := b.unfollow(target.username); err != nil {
			log.Errorf("unfollow target error: %v", err)
			continue
		}
		log.Printf("unfollowed target %q\n", target.username)
		b.targets[target.username].deleted = true
		randomSleep()
	}

	log.Println("done unfollowing all followed targets")
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

	w.Flush()

	if err := w.Error(); err != nil {
		return err
	}

	log.Println("done saving targets to file")
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

	w.Flush()

	if err := w.Error(); err != nil {
		return err
	}

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

	w.Flush()

	if err := w.Error(); err != nil {
		return err
	}

	return nil
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
	log.Printf("searching users with %q\n", query)
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
		log.Printf("fetching %v users for term %q", lastSize, query)
		collection = append(collection, result.Users...)
	}

	return collection, nil
}

func (b *Bot) searchActiveUsers(queries []string) error {
	log.Println("starting searching for active users")
	for _, query := range queries {
		query := strings.TrimSpace(query)
		if query == "" {
			continue
		}
		users, err := b.searchUsers(query, 100)
		if err != nil {
			return err
		}

		var wg sync.WaitGroup
		for _, user := range users {
			wg.Add(1)
			go func(user github.User) {
				defer wg.Done()
				username := user.GetLogin()
				_, ok := b.originalFollowing[username]
				if ok {
					return
				}

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

		log.Printf("found %v active targets\n", len(b.targets))
	}

	log.Println("done searching for active users")
	return nil
}

func (b *Bot) handleExitSignal() {
	var gracefulStop = make(chan os.Signal)
	signal.Notify(gracefulStop, syscall.SIGTERM)
	signal.Notify(gracefulStop, syscall.SIGINT)
	go func() {
		sig := <-gracefulStop
		log.Printf("caught signal: %+v\n", sig)
		log.Println("saving latest state")
		if err := b.saveTargets(); err != nil {
			panic(err)
		}
		os.Exit(0)
	}()
}

func randomSleep() {
	i := randomInt(1, 7)
	time.Sleep(time.Duration(i) * time.Second)
}

func randomInt(min, max int) int {
	return rand.Intn(max-min) + min
}

func userHomeDir() string {
	if runtime.GOOS == "windows" {
		home := os.Getenv("HOMEDRIVE") + os.Getenv("HOMEPATH")
		if home == "" {
			home = os.Getenv("USERPROFILE")
		}
		return home
	} else if runtime.GOOS == "linux" {
		home := os.Getenv("XDG_CONFIG_HOME")
		if home != "" {
			return home
		}
	}
	return os.Getenv("HOME")
}

func normalizePath(path string) string {
	// expand tilde
	if strings.HasPrefix(path, "~/") {
		path = filepath.Join(userHomeDir(), path[2:])
	}

	return path
}

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}
