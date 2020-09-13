package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/sheets/v4"

	twitterscraper "github.com/n0madic/twitter-scraper"
	"github.com/spf13/viper"
	"golang.org/x/net/context"

	"github.com/polynomialspace/twitterimg2telegram/gsheets"
	"github.com/polynomialspace/twitterimg2telegram/secrets"
)

type Twitter struct {
	Users []struct {
		User   string `mapstructure:"user"`
		Tweets string `mapstructure:"tweets"`
		Last   int64  `mapstructure:"last"`
	} `mapstructure:"users"`
	PollTime int `mapstructure:"polltime"`
}
type Telegram struct {
	Channels string `mapstructure:"channels"`
	ApiKey   string `mapstructure:"apikey"`
}
type Google struct {
	SheetsID      string `mapstructure:"sheets_id"`
	SheetsRange   string `mapstructure:"sheets_range"`
	SecretsProjID string `mapstructure:"secrets_project_id,omitempty"`
}

type Config struct {
	Twitter  `mapstructure:"twitter"`
	Telegram `mapstructure:"telegram"`
	Google   `mapstructure:"google,omitempty"`
	Stateful bool `mapstructure:"-"` //false: run once, gsheets to store state. true: loop and writeback to config.yml
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

func main() {
	/* config */
	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	viper.SetConfigType("yml")

	cfg := Config{}
	viper.AutomaticEnv()
	viper.SetEnvPrefix("TW2TG")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalln(err)
	}
	if err := viper.Unmarshal(&cfg); err != nil {
		log.Fatalln(err)
	}

	if cfg.Google.SecretsProjID == "" || cfg.Google.SheetsID == "" {
		cfg.Stateful = true
	}

	var srv *sheets.Service
	if !cfg.Stateful {
		sheetsKey, err := secrets.Get(cfg.Google.SecretsProjID, "client_secret_json")
		if err != nil {
			log.Fatalln(err)
		}

		srv, err = gsheets.NewSheet(sheetsKey)
		if err != nil {
			log.Fatalln(err)
		}
		readRange := cfg.Google.SheetsRange
		resp, err := srv.Spreadsheets.Values.Get(cfg.Google.SheetsID, readRange).Do()

		if err != nil {
			log.Fatalf("Unable to retrieve data from sheet: %v", err)
		}

		if len(resp.Values) == 0 {
			log.Println("No data found.")
		}
		for _, row := range resp.Values {
			for iuser, user := range cfg.Twitter.Users {
				if row[0] == user.User {
					val, err := strconv.Atoi(row[1].(string))
					if err != nil {
						continue
					}
					if int64(val) <= user.Last {
						continue
					}
					log.Printf("Updated last poll time for '%s' from '%v' to '%v'\n",
						user.User, user.Last, val)
					cfg.Twitter.Users[iuser].Last = int64(val)
				}
			}
		}
	}

	/* Twitter scraper loop */
	c := make(chan SendPhoto)
	go scrapeTwitter(c, cfg, srv)

	/* Telegram posting runner */
	message := new(bytes.Buffer)
	encoder := json.NewEncoder(message)
	encoder.SetEscapeHTML(false)
	telegramAPIURL := `https://api.telegram.org/bot` + cfg.Telegram.ApiKey
	for photo := range c {
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

	/* GCR healthcheck ... */
	go func() {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}

		log.Printf("Listening on port %s to make GCR happy : )", port)
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, "ok")
		})
		log.Fatal(http.ListenAndServe(":"+port, nil))
	}()

}

func scrapeTwitter(c chan<- SendPhoto, cfg Config, srv *sheets.Service) {
	readRange := cfg.Google.SheetsRange // XXX
	sheetValues := [][]interface{}{{"Account", "Last Updated"}}
	/* should infloop only if srv == nil (ie not using gsheets) */
	for loop := true; loop; {
		for iuser, user := range cfg.Twitter.Users {
			tweets := make([]*twitterscraper.Result, 0)
			for tweet := range twitterscraper.GetTweets(context.Background(), user.User, 32) {
				tweets = append(tweets, tweet)
			}

			for i := len(tweets) - 1; i >= 0; i-- {
				tweet := tweets[i]
				if err := tweet.Error; err != nil {
					log.Println(err)
				}
				if tweet.Timestamp <= user.Last {
					continue
				}
				cfg.Twitter.Users[iuser].Last = tweet.Timestamp

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
			sheetValues = append(sheetValues, []interface{}{
				cfg.Twitter.Users[iuser].User, cfg.Twitter.Users[iuser].Last,
			})

		}

		if srv != nil { // XXX this should be cfg.Stateful
			_, err := srv.Spreadsheets.Values.Update(cfg.Google.SheetsID, readRange,
				&sheets.ValueRange{
					Values: sheetValues,
				}).ValueInputOption("USER_ENTERED").Do() //RAW?
			if err != nil {
				log.Fatalf("Unable to retrieve data from sheet: %v", err)
			}
			// XXX only update if values changed
			log.Println("Wrote values back to GSheets. Exiting.")
			loop = false
		} else {
			log.Printf("Updating config.yaml and waiting %d seconds.\n",
				cfg.Twitter.PollTime)
			viper.Set("twitter.users", cfg.Twitter.Users)
			err := viper.WriteConfig()
			if err != nil {
				log.Fatalln(err)
			}
			time.Sleep(time.Second * time.Duration(cfg.Twitter.PollTime))
		}
	}

	close(c)
}
