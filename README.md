# rss2telegram
RSS feed for Telegram using Google Cloud Functions

## Deployment
Copy `.env.example.yaml` to `.env.yaml` and put your values in there.
```
gcloud functions deploy RSS2Telegram --env-vars-file .env.yaml --runtime go113 --trigger-topic RSS2Telegram
```

## Testing
```
gcloud pubsub topics publish RSS2Telegram --message ' '
```

## Local Development
Set environemnt variables:
 - `RSS_FEED_URL`
 - `TELEGRAM_BOT_API_TOKEN`
 - `TELEGRAM_CHAT_ID`
 - `GCP_PROJECT`

Then run:
```bash
go run ./cmd/main.go
```