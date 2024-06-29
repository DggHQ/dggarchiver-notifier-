# dggarchiver-notifier
This is the notifier service of the dggarchiver service that triggers download workers to download a livestream.

## Features

1. Supported livestream platforms:
   - YouTube (Web scraping or API)
   - Rumble (Web scraping)
   - Kick (API scraping)
2. Platform priority option (able to ignore other platforms if there's already a stream from a prioritised platform)
3. Lua plugin support

## Lua

The service can be extended with Lua plugins/scripts. An example can be found in the ```notifier.example.lua``` file.

If enabled, the service will call these functions from the specified ```.lua``` file:
- ```OnReceive(vod)``` when a livestream has been found, where ```vod``` is the livestream ID
- ```OnSend(vod)``` when a livestream has been sent to the ```<topic>.job``` NATS topic, where ```vod``` is the livestream struct

After the functions are done executing, the service will check the global ```ReceiveResponse``` and ```SendResponse``` variables for errors, before returning the struct. The struct's fields are:
```go
type LuaResponse struct {
	Filled  bool
	Error   bool
	Message string
	Data    map[string]interface{}
}
```

## Configuration

The config file location can be set with the ```CONFIG``` environment variable. Example configuration can be found below and in the ```config.example.yaml``` file.

```yaml
notifier:
  platforms:
    youtube:
      enabled: yes
      method: scraper # mandatory field, parsing method, will default to 'scraper', can be set to 'api' or 'scraper'
      downloader: ytarchive # optional field, will default to yt-dlp, can be set to either 'yt-dlp', 'yt-dlp/piped' or 'ytarchive'
      restream_priority: 1 # optional field, sets the platform priority (ignore if there's already a stream going from a higher priority platform)
      google_credentials: client_secret.json # mandatory field, google credentials file with enabled YouTube Data API
      channel: UCSJ4gkVC6NrvII8umztf0Ow # mandatory field, YouTube channel ID
      refresh_time: 5 # mandatory field, livestream check time in minutes, set to 0 to disable
      healthcheck: https://hc-ping.com/your-uuid-here # optional field, healthcheck URL
    rumble:
      enabled: yes
      downloader: yt-dlp # optional field, only yt-dlp supported for now
      restream_priority: 3 # optional field, sets the platform priority (ignore if there's already a stream going from a higher priority platform)
      channel: Destiny # mandatory field, Rumble channel ID
      refresh_time: 5 # mandatory field, livestream check time in minutes
      healthcheck: https://hc-ping.com/your-uuid-here # optional field, healthcheck URL
    kick:
      enabled: yes
      downloader: N_m3u8DL-RE # optional field, will default to yt-dlp, can be set to either 'yt-dlp' or 'N_m3u8DL-RE'
      restream_priority: 2 # optional field, sets the platform priority (ignore if there's already a stream going from a higher priority platform)
      channel: destiny # mandatory field, Kick channel ID
      refresh_time: 5 # mandatory field, livestream check time in minutes
      healthcheck: https://hc-ping.com/your-uuid-here # optional field, healthcheck URL
      proxy_url: http://proxy:80 # optional field, proxy url in case kick is being cringe
  plugins:
    enabled: no
    path: ./notifier.lua # path to the lua plugin
  verbose: no # increases log verbosity

nats:
  host: nats # nats uri
  topic: archiver # main nats topic
```