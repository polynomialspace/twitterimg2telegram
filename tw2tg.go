package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	twitterscraper "github.com/n0madic/twitter-scraper"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Twitter struct {
		Users    []string `yaml:"users"`
		Tweets   string   `yaml:"tweets"`
		PollTime int      `yaml:"poll"`
		Last     []int64  `yaml:"last"`
	} `yaml:"twitter"`
	Telegram struct {
		Channels string `yaml:"channels"`
		ApiKey   string `yaml:"apikey"`
	} `yaml:"telegram"`
}

func (c *Config) FromFile(path string) error {
	cfgData, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(cfgData, c)
	return err
}

func (c *Config) ToFile(path string) error {
	cfgData, err := yaml.Marshal(c)
	if err != nil {
		log.Println(err)
	}
	err = ioutil.WriteFile(path, cfgData, 0600)
	return err
}

type SendPhoto struct {
	Recipient string `json:"chat_id"`
	Photo     string `json:"photo"`
	Caption   string `json:"caption,omitempty"`
}

type Response struct {
	Ok          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
}

const (
	cfgPath = "./config.yml"
)

func main() {
	cfg := Config{}
	cfg.FromFile(cfgPath)

	/* write back config on ^C or other exit */
	/* (mostly to save last tweet timestamp) */
	writeBack := func() {
		log.Println("Writing back config to:", cfgPath)
		cfg.ToFile(cfgPath)
	}
	defer writeBack()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		s := <-sig
		log.Println("Received", s)
		writeBack()
		os.Exit(1)
	}()

	c := make(chan SendPhoto)

	go func() {
		for iuser, user := range cfg.Twitter.Users {
			tweets := make([]*twitterscraper.Result, 0)
			for tweet := range twitterscraper.GetTweets(context.Background(), user, 16) {
				tweets = append(tweets, tweet)
			}

			for i := len(tweets) - 1; i >= 0; i-- {
				tweet := tweets[i]
				if err := tweet.Error; err != nil {
					log.Println(err)
				}
				if tweet.Timestamp <= cfg.Twitter.Last[iuser] {
					continue
				}
				cfg.Twitter.Last[iuser] = tweet.Timestamp

				for _, photo := range tweet.Photos {
					log.Printf("<-  Twitter: @%s %s\n", tweet.Username, tweet.PermanentURL)
					c <- SendPhoto{
						cfg.Telegram.Channels,
						photo,
						tweet.PermanentURL,
					}
				}
			}
			tweets = tweets[:0]

			time.Sleep(time.Second * time.Duration(cfg.Twitter.PollTime))
		}
	}()

	message := new(bytes.Buffer)
	encoder := json.NewEncoder(message)
	encoder.SetEscapeHTML(false)
	telegramAPIURL := `https://api.telegram.org/bot` + cfg.Telegram.ApiKey

	for {
		photo := <-c
		encoder.Encode(photo)
		log.Println("-> Telegram:", photo.Recipient, photo.Caption)
		res, err := http.Post(telegramAPIURL+"/sendPhoto", "application/json", message)
		if err != nil {
			log.Fatalln(err)
		}
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			log.Fatalln(err)
		}
		tgResponse := Response{}

		err = json.Unmarshal(body, &tgResponse)
		if err != nil {
			log.Fatalln(err)
		}
		if !tgResponse.Ok {
			log.Fatalln(tgResponse.Description)
		}

	}

}
