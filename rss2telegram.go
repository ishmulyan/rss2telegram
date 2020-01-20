package rss2telegram

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/firestore"
	md "github.com/Skarlso/html-to-markdown"
	"github.com/mmcdole/gofeed"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	// projectID is set from the GCP_PROJECT environment variable, which is
	// automatically set by the Cloud Functions runtime.
	projectID = os.Getenv("GCP_PROJECT")
	// client is a global Firestore client, initialized once per instance.
	client    *firestore.Client
	converter = md.NewConverter("", true, &md.Options{
		StrongDelimiter: "*",
	})
)

func init() {
	// err is pre-declared to avoid shadowing client.
	var err error

	// client is initialized with context.Background() because it should
	// persist between function invocations.
	client, err = firestore.NewClient(context.Background(), projectID)
	if err != nil {
		log.Fatalf("firestore.NewClient: %v", err)
	}
}

// PubSubMessage is the payload of a Pub/Sub event.
type PubSubMessage struct{}

// RSS2Telegram is a background cloud function that retrives RSS feed and post updates to telegram.
// Uses such environment variables:
// - RSS_FEED_URL
// - TELEGRAM_BOT_API_TOKEN
// - TELEGRAM_CHAT_ID
func RSS2Telegram(ctx context.Context, m PubSubMessage) error {
	rssFeedURL := os.Getenv("RSS_FEED_URL")
	if rssFeedURL == "" {
		return errors.New("environment variable RSS_FEED_URL not set")
	}
	tBotAPIToken := os.Getenv("TELEGRAM_BOT_API_TOKEN")
	if tBotAPIToken == "" {
		return errors.New("environment variable TELEGRAM_BOT_API_TOKEN not set")
	}
	tChatID := os.Getenv("TELEGRAM_CHAT_ID")
	if tChatID == "" {
		return errors.New("environment variable TELEGRAM_CHAT_ID not set")
	}

	// create new feed parser and parse provided rss feed url
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(rssFeedURL)
	if err != nil {
		return err
	}

	// read the previous published time of the feed from firestore
	publishedAt, err := readPublishedAt(ctx, client, tChatID, rssFeedURL)
	if err != nil {
		return err
	}

	var newPublishedAt time.Time

	// iterate over feed in reverse order so processing is from older to newer
	for i := len(feed.Items) - 1; 0 <= i; i-- {
		if feed.Items[i].PublishedParsed == nil {
			// skip items without pubslied time
			continue
		}

		if !feed.Items[i].PublishedParsed.After(publishedAt) {
			// skip item that was published before the previous published time of the feed
			continue
		}

		newPublishedAt = *feed.Items[i].PublishedParsed

		if err := sendToTelegram(tBotAPIToken, tChatID, feed.Items[i]); err != nil {
			log.Println(err)
		}
	}

	if !newPublishedAt.IsZero() {
		// write the feed published time to firestore
		if err := writePublishedAt(ctx, client, tChatID, rssFeedURL, newPublishedAt); err != nil {
			return err
		}
	}

	return nil
}

// readPublishedAt reads the time rssURL feed was published to telegram chat chatID from firestore.
func readPublishedAt(ctx context.Context, client *firestore.Client, chatID, rssURL string) (time.Time, error) {
	dsnap, err := client.Collection("chats").Doc(chatID).Get(ctx)
	if status.Code(err) == codes.NotFound {
		// collection or doc not found, feed was never published
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}

	data, err := dsnap.DataAtPath([]string{"publishedAt", rssURL})
	if err != nil {
		// data at path "publishedAt" not found, feed was never published
		return time.Time{}, nil
	}

	t, ok := data.(time.Time)
	if !ok {
		// data is not time.Time, return zero time.Time as a default value
		return time.Time{}, nil
	}

	return t, nil
}

// writePublishedAt writes the time rssURL feed was published to telegram chat chatID from firestore.
func writePublishedAt(ctx context.Context, client *firestore.Client, chatID, rssURL string, t time.Time) error {
	doc := client.Collection("chats").Doc(chatID)
	_, err := doc.Update(ctx, []firestore.Update{{
		FieldPath: []string{"publishedAt", rssURL},
		Value:     t,
	}})

	if err != nil {
		if status.Code(err) == codes.NotFound {
			// collection or doc not found, create a doc
			_, err = doc.Set(ctx, map[string]interface{}{
				"publishedAt": map[string]interface{}{
					rssURL: t,
				},
			})
		}

		if err != nil {
			return err
		}
	}

	return nil
}

func sendToTelegram(botAPIToken, chatID string, item *gofeed.Item) error {
	content, err := converter.ConvertString(item.Content)
	if err != nil {
		log.Println(err)
		content = item.Content
	}

	text := fmt.Sprintf("*%s*\n\n%s", item.Title, content)

	resp, err := http.PostForm(fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botAPIToken), map[string][]string{
		"chat_id":                  {chatID},
		"text":                     {text},
		"parse_mode":               {"markdown"},
		"disable_web_page_preview": {"true"},
	})
	if err != nil {
		return err
	}

	data, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("status code: %d, data: %s", resp.StatusCode, data)
	}

	return nil
}
