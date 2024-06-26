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
	"sync"
)

type Subscription struct {
	Url         string
	Subscribers []int64
	Data        []string
}

func hrefScraper(url, selector, baseUrl string) (scraped []string, _ error) {
	c := colly.NewCollector()
	c.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:122.0) Gecko/20100101 Firefox/122.0"
	c.OnHTML(selector, func(e *colly.HTMLElement) {
		scraped = append(scraped, baseUrl+e.Attr("href"))
	})
	return scraped, c.Visit(url)
}

var wg sync.WaitGroup

func scrapeAndUpdate(bot *telego.Bot, pool *pgxpool.Pool, sub Subscription) {
	defer wg.Done()
	var selector, url string
	switch {
	case strings.Contains(sub.Url, "djinni.co"):
		selector, url = "a[class*=\" job-list\"]", "https://djinni.co"
	case strings.Contains(sub.Url, "jobs.dou.ua"):
		selector = "a.vt"
	case strings.Contains(sub.Url, "nofluffjobs.com"):
		selector, url = "nfj-postings-list[listname=\"search\"] a", "https://nofluffjobs.com"
	default:
		bot.Logger().Errorf("Unknown url: %v", sub.Url)
		return
	}
	scraped, err := hrefScraper(sub.Url, selector, url)
	if err != nil {
		bot.Logger().Errorf("Failed to scrape: %v, %v", sub.Url, err)
		return
	}

	newScraps := 0

	for _, scr := range scraped {
		if !slices.Contains(sub.Data, scr) {
			for _, id := range sub.Subscribers {
				_, err = bot.SendMessage(&telego.SendMessageParams{
					ChatID: telego.ChatID{ID: id},
					Text:   scr,
				})
				if err != nil {
					bot.Logger().Errorf("Failed to send message: %v, %v", scr, err)
				}
			}
			newScraps++
		}
	}

	// if no new data, don't bother updating
	if newScraps == 0 {
		return
	}

	_, err = pool.Exec(context.Background(), `update subscription set data = $1 where url = $2`, scraped, sub.Url)
	if err != nil {
		bot.Logger().Errorf("Failed to update subscription: %v, %v", sub.Url, err)
	}
}

func main() {
	bot, err := telego.NewBot(os.Getenv("TELEGRAM_BOT_TOKEN"))
	if err != nil {
		fmt.Println("error creating bot:", err)
		return
	}

	pool, err := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		bot.Logger().Errorf("error connecting to postgres: %v", err)
		return
	}

	cursor, err := pool.Query(context.Background(), "select * from subscription")
	if err != nil {
		bot.Logger().Errorf("error getting subs: %v", err)
		return
	}

	for cursor.Next() {
		s := Subscription{}
		if err = cursor.Scan(&s.Url, &s.Data, &s.Subscribers); err != nil || s.Url == "" || len(s.Subscribers) == 0 {
			bot.Logger().Errorf("invalid Subscription: %v, %v", err, s)
			continue
		}

		wg.Add(1)
		go scrapeAndUpdate(bot, pool, s)
	}

	cursor.Close()
	wg.Wait()
	pool.Close()
}
