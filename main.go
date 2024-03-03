package main

import (
	"context"
	"fmt"
	"github.com/gocolly/colly"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mymmrac/telego"
	"os"
	"slices"
	"strings"
	"time"
)

type sub struct {
	Url         string   `json:"url"`
	Subscribers []int64  `json:"subscribers"`
	Data        []string `json:"data"`
}

func hrefScraper(url, selector, baseUrl string) (scraped []string) {
	c := colly.NewCollector()
	c.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:122.0) Gecko/20100101 Firefox/122.0"
	c.OnHTML(selector, func(e *colly.HTMLElement) {
		scraped = append(scraped, baseUrl+e.Attr("href"))
	})
	err := c.Visit(url)
	if err != nil {
		fmt.Printf("Failed to scrape %s %v\n", err, url)
	}
	return
}

func scrapeAndUpdate(bot *telego.Bot, pool *pgxpool.Pool, s sub) {
	var selector, url string
	switch {
	case strings.Contains(s.Url, "djinni.co"):
		selector, url = "a[class*=\" job-list\"]", "https://djinni.co"
	case strings.Contains(s.Url, "jobs.dou.ua"):
		selector = "a.vt"
	case strings.Contains(s.Url, "nofluffjobs.com"):
		selector, url = "nfj-postings-list[listname=\"search\"] a", "https://nofluffjobs.com"
	default:
		return
	}
	scraped := hrefScraper(s.Url, selector, url)

	newScraps := 0

	for _, scr := range scraped {
		if !slices.Contains(s.Data, scr) {
			for _, id := range s.Subscribers {
				go bot.SendMessage(&telego.SendMessageParams{
					ChatID: telego.ChatID{ID: id},
					Text:   scr,
				})
			}
			newScraps++
		}
	}

	// if no new data, don't bother updating
	if newScraps == 0 {
		return
	}

	_, err := pool.Exec(context.Background(), `update subscription set data = $1 where url = $2`, scraped, s.Url)
	if err != nil {
		bot.Logger().Errorf("Failed to update subscription: %v, %v", s.Url, err)
	}
}

func main() {
	pool, err := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		fmt.Println("error connecting to postgres:", err)
		return
	}

	bot, err := telego.NewBot(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if err != nil {
		fmt.Println("error creating bot:", err)
		return
	}

	cursor, err := pool.Query(context.Background(), "select * from subscription")
	if err != nil {
		bot.Logger().Errorf("error getting subs: %v", err)
		return
	}

	for cursor.Next() {
		var s sub
		if err = cursor.Scan(&s.Url, &s.Data, &s.Subscribers); err != nil || s.Url == "" || len(s.Subscribers) == 0 {
			bot.Logger().Errorf("invalid sub: %v, %v", err, s)
			continue
		}

		go scrapeAndUpdate(bot, pool, s)
	}

	cursor.Close()
	if err != nil {
		bot.Logger().Errorf("error closing cursor: %v", err)
	}
	time.Sleep(10 * time.Second)
	pool.Close()
}
