package main

import (
	"context"
	"fmt"
	"github.com/mymmrac/telego"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"os"
	"regexp"
	"slices"
	"time"
)

type sub struct {
	Url         string   `json:"url"`
	Subscribers []int64  `json:"subscribers"`
	Data        []string `json:"data"`
}

var djinniRegex = regexp.MustCompile(`https://djinni\.co/jobs/.*`)

func (s sub) getCrawlerAndSelector() (crawler, string) {
	if djinniRegex.MatchString(s.Url) {
		return djinniCrawler, ".list-unstyled"
	} else {
		return douCrawler, ".lt"
	}
}

func scrapeAndUpdate(bot *telego.Bot, col *mongo.Collection, s sub) {
	cr, selector := s.getCrawlerAndSelector()
	scraped := htmlUlScraper(s.Url, selector, cr)

	for _, scr := range scraped {
		if !slices.Contains(s.Data, scr) {
			for _, id := range s.Subscribers {
				go bot.SendMessage(&telego.SendMessageParams{
					ChatID: telego.ChatID{ID: id},
					Text:   scr,
				})
			}
		}
	}

	s.Data = scraped
	_, err := col.UpdateOne(context.Background(), bson.M{"url": s.Url}, bson.M{"$set": s}, options.Update().SetUpsert(true))
	if err != nil {
		fmt.Println("Failed to update subscription", s.Url, err)
	}
}

func main() {
	db, err := mongo.Connect(context.Background(), options.Client().ApplyURI(os.Getenv("MONGO_URL")))
	if err != nil {
		fmt.Println("error connecting to mongo:", err)
		return
	}
	col := db.Database("job-scraper").Collection("subscriptions")

	bot, err := telego.NewBot(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if err != nil {
		fmt.Println("error creating bot:", err)
		return
	}

	cursor, err := col.Find(context.Background(), bson.D{})
	if err != nil {
		fmt.Println("error finding:", err)
		return
	}
	defer func() {
		err = cursor.Close(context.Background())
		if err != nil {
			fmt.Println("error closing cursor:", err)
		}
	}()

	for cursor.Next(context.Background()) {
		var s sub
		if err = cursor.Decode(&s); err != nil || s.Url == "" || len(s.Subscribers) == 0 {
			fmt.Println("invalid sub:", err, s)
			continue
		}

		go scrapeAndUpdate(bot, col, s)
	}

	time.Sleep(10 * time.Second)
}
