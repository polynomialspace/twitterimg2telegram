Really low effort bot that takes images posted from a twitter account and posts them on telegram.

config.yaml should be something like
```yaml
twitter:
  users:
  - TwitterUser
  tweets: photos
  poll: 60
  last:
  - 0
telegram:
  channels: @Channel
  apikey: TelegramBotAPIKey
  ```
