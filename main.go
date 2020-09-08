package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/fatih/color"
	"github.com/kurrik/oauth1a"
	"github.com/kurrik/twittergo"
)

type config struct {
	APIKey            string   `toml:"APIKey"`
	APISecretKey      string   `toml:"APISecretKey"`
	AccessToken       string   `toml:"AccessToken"`
	AccessTokenSecret string   `toml:"AccessTokenSecret"`
	Protect           []string `toml:"Protect"`
}

func loadConfigFrom(configFile string) (client *twittergo.Client, config *config, err error) {
	if _, err := toml.DecodeFile(configFile, &config); err != nil {
		log.Fatal(err)
	}

	UserConfig := oauth1a.NewAuthorizedConfig(config.AccessToken, config.AccessTokenSecret)
	ClientConfig := &oauth1a.ClientConfig{
		ConsumerKey:    config.APIKey,
		ConsumerSecret: config.APISecretKey,
	}
	client = twittergo.NewClient(ClientConfig, UserConfig)
	return
}

func verifyCredentials(client *twittergo.Client) (user *twittergo.User, err error) {

	var (
		req  *http.Request
		resp *twittergo.APIResponse
	)

	req, err = http.NewRequest("GET", "/1.1/account/verify_credentials.json", nil)
	resp, err = client.SendRequest(req)
	if err != nil {
		return
	}

	user = &twittergo.User{}
	err = resp.Parse(user)
	return
}

func main() {
	var (
		err     error
		client  *twittergo.Client
		req     *http.Request
		resp    *twittergo.APIResponse
		query   url.Values
		results *twittergo.Timeline
		config  *config
	)

	cyan := color.New(color.FgCyan)
	yellow := color.New(color.FgYellow)
	red := color.New(color.FgRed)

	if client, config, err = loadConfigFrom(os.Args[1]); err != nil {
		log.Fatalf("Could not parse config file: %v\n", err)
	}

	user, err := verifyCredentials(client)
	if err != nil {
		log.Println(err)
	}

	const (
		count   int = 200
		urltmpl     = "/1.1/statuses/user_timeline.json?%v"
		minwait     = time.Duration(10) * time.Second
	)

	query = url.Values{}
	query.Set("count", fmt.Sprintf("%v", count))
	query.Set("screen_name", user.ScreenName())

	total := 0
	protected := 0
	last := ""
	end := false

	for {
		endpoint := fmt.Sprintf(urltmpl, query.Encode())
		if req, err = http.NewRequest("GET", endpoint, nil); err != nil {
			log.Fatalf("Could not parse request: %v\n", err)
		}
		if resp, err = client.SendRequest(req); err != nil {
			log.Fatalf("Could not send request: %v\n", err)
		}
		results = &twittergo.Timeline{}
		if err = resp.Parse(results); err != nil {
			if rle, ok := err.(twittergo.RateLimitError); ok {
				dur := rle.Reset.Sub(time.Now()) + time.Second
				if dur < minwait {
					dur = minwait
				}
				msg := "Rate limited. Reset at %v. Waiting for %v\n"
				fmt.Printf(msg, rle.Reset, dur)
				time.Sleep(dur)
				continue
			} else {
				fmt.Printf("Problem parsing response: %v\n", err)
			}
		}
		batch := len(*results)
		if batch == 0 {
			break
		} else if end {
			break
		}
		for _, tweet := range *results {
			if last == tweet.IdStr() {
				end = true
				break
			}
			last = tweet.IdStr()
			if contains(config.Protect, tweet.IdStr()) {
				yellow.Printf("[Skipped] ")
				fmt.Println(tweet.IdStr())
				protected++
				continue
			} else {
				endpoint := "/1.1/statuses/destroy/" + strconv.FormatUint(tweet.Id(), 10) + ".json"
				data := url.Values{}
				data.Set("id", strconv.FormatUint(tweet.Id(), 10))
				body := strings.NewReader(data.Encode())
				req, err = http.NewRequest("POST", endpoint, body)
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				if err != nil {
					log.Fatalln(err)
				}
				resp, err = client.SendRequest(req)
				if err != nil {
					log.Fatalf("Could not send request: %v\n", err)
				}
				cyan.Printf("[Deleted] ")
				fmt.Println(tweet.IdStr())

				total++
			}
		}
	}
	red.Printf("%d tweets are deleted and %d tweets are protected.\n", total, protected)
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}
