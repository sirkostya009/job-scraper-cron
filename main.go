package main

import (
	"context"
	"fmt"
	"github.com/gocolly/colly"
	"github.com/mymmrac/telego"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"os"
	"regexp"
	"slices"
	"strings"
)

type crawler func(e *colly.HTMLElement) string

func djinniCrawler(e *colly.HTMLElement) string {
	title := e.ChildText("div header div.mb-1 a")
	title = strings.ReplaceAll(title, "\n", "")
	metadata := make([]string, 0, 4)
	e.ForEach("div header div.font-weight-500 span.nobr", func(i int, e *colly.HTMLElement) {
		metadata = append(metadata, strings.ReplaceAll(e.Text, "\n ", ""))
	})
	url := "https://djinni.co" + e.ChildAttr("div header div.mb-1 div a", "href")
	return fmt.Sprintf("%s%s\n%s", title, strings.Join(metadata, ","), url)
}

func douCrawler(e *colly.HTMLElement) string {
	title := e.ChildText("div.title a")
	locations := e.ChildText("div.title span")
	url := e.ChildAttr("div.title a", "href")
	return fmt.Sprintf("%s %s\n%s", title, locations, url)
}

func htmlUlScraper(url, selector string, callback crawler) (scraped []string) {
	c := colly.NewCollector(colly.UserAgent(
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/109.0.0.0 Safari/537.36",
	))
	c.OnHTML(selector, func(e *colly.HTMLElement) {
		e.ForEach("li", func(i int, e *colly.HTMLElement) {
			scraped = append(scraped, callback(e))
		})
	})
	err := c.Visit(url)
	if err != nil {
		fmt.Printf("Failed to scrape %s\n%v\n", url, err)
	}
	return
}

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
		_, err = col.UpdateOne(context.Background(), bson.M{"url": s.Url}, bson.M{"$set": s}, options.Update().SetUpsert(true))
		if err != nil {
			fmt.Println("Failed to update subscription", s.Url, err)
		}
	}
}
