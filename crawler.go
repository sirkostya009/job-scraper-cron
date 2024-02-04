package main

import (
	"fmt"
	"github.com/gocolly/colly"
)

type crawler func(e *colly.HTMLElement) string

func djinniCrawler(e *colly.HTMLElement) string {
	return "https://djinni.co" + e.ChildAttr("div header div.mb-1 div a", "href")
}

func douCrawler(e *colly.HTMLElement) string {
	return e.ChildAttr("div.title a", "href")
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
