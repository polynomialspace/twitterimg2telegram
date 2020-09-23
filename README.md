Really low effort bot that takes images posted from a twitter account and posts them on telegram.

config.yaml should be something like
```yaml
telegram:
  apikey: TELEGRAM_API_KEY
  channels: '@TELEGRAM_CHANNEL'
twitter:
  polltime: 1800
  users:
  - user: twitter_user
    tweets: photos
    last: 1600786105
  - user: twitter_user_2
    tweets: photos
    last: 1600847095
  ```
