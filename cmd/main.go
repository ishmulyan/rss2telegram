package main

import (
	"context"
	"log"

	"github.com/ishmulyan/rss2telegram"
)

func main() {
	if err := rss2telegram.RSS2Telegram(context.Background(), rss2telegram.PubSubMessage{}); err != nil {
		log.Fatal(err)
	}
}
