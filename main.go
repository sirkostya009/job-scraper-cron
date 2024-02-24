package main

import (
	"context"
	"fmt"
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

func (s sub) getCrawlerAndSelector() (crawler, string) {
	switch {
	case strings.Contains(s.Url, "djinni.co"):
		return djinniCrawler, ".list-unstyled"
	case strings.Contains(s.Url, "jobs.dou.ua"):
		return douCrawler, ".lt"
	default:
		return nil, ""
	}
}

func equalNoOrder(s []string, e []string) bool {
	for _, a := range s {
		if !slices.Contains(e, a) {
			return false
		}
	}
	return true
}

func scrapeAndUpdate(bot *telego.Bot, pool *pgxpool.Pool, s sub) {
	cr, selector := s.getCrawlerAndSelector()
	scraped := htmlUlScraper(s.Url, selector, cr)

	// if scraped data is semantically the same as the last scraped data, do nothing
	if equalNoOrder(scraped, s.Data) {
		return
	}

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
	_, err := pool.Exec(context.Background(), `
		update subscription
		set data = $1
		where url = $2
	`, s.Data, s.Url)
	if err != nil {
		bot.Logger().Errorf("Failed to update subscription: %v, %v", s.Url, err)
	}
}

func main() {
	pool, err := pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		fmt.Println("error connecting to mongo:", err)
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
