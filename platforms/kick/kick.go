package kick

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"

	config "github.com/DggHQ/dggarchiver-config/notifier"
	dggarchivermodel "github.com/DggHQ/dggarchiver-model"
	"github.com/DggHQ/dggarchiver-notifier/platforms/implementation"
	"github.com/DggHQ/dggarchiver-notifier/state"
	"github.com/DggHQ/dggarchiver-notifier/util"
	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	lua "github.com/yuin/gopher-lua"
)

const (
	platformName   string = "kick"
	platformMethod string = "scraper"
)

func init() {
	implementation.Map[fmt.Sprintf("%s_%s", platformName, platformMethod)] = New
}

type Platform struct {
	httpClient tls_client.HttpClient
	cfg        *config.Config
	state      *state.State
	prefix     slog.Attr
	sleepTime  time.Duration
}

type api struct {
	URL        string `json:"playback_url"`
	Livestream struct {
		IsLive    bool   `json:"is_live"`
		ID        int    `json:"id"`
		Slug      string `json:"slug"`
		CreatedAt string `json:"created_at"`
		Title     string `json:"session_title"`
		Thumbnail struct {
			URL string `json:"responsive"`
		} `json:"thumbnail"`
	} `json:"livestream"`
}

// New returns a new Kick platform struct
func New(cfg *config.Config, state *state.State) implementation.Platform {
	var err error

	p := Platform{
		cfg:   cfg,
		state: state,
		prefix: slog.Group("platform",
			slog.String("name", platformName),
			slog.String("method", platformMethod),
		),
		sleepTime: time.Second * 60 * time.Duration(cfg.Platforms.Kick.RefreshTime),
	}

	jar := tls_client.NewCookieJar()
	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(30),
		tls_client.WithClientProfile(profiles.Chrome_110),
		tls_client.WithNotFollowRedirects(),
		tls_client.WithCookieJar(jar),
	}

	if cfg.Platforms.Kick.ProxyURL != "" {
		options = append(options, tls_client.WithProxyUrl(cfg.Platforms.Kick.ProxyURL))
	}

	p.httpClient, err = tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
	if err != nil {
		slog.Error("unable to create a TLS client",
			p.prefix,
			slog.Any("err", err),
		)
		os.Exit(1)
	}

	return &p
}

// GetPrefix returns a slog.Attr group for platform p
func (p *Platform) GetPrefix() slog.Attr {
	return p.prefix
}

// GetSleepTime returns sleep duration for platform p
func (p *Platform) GetSleepTime() time.Duration {
	return p.sleepTime
}

// CheckLivestream checks for an existing livestream on platform p,
// and, if found, publishes the info to NATS
func (p *Platform) CheckLivestream(l *lua.LState) error {
	stream := p.scrape()

	if stream != nil && stream.Livestream.IsLive {
		if !slices.Contains(p.state.SentVODs, fmt.Sprintf("kick:%d", stream.Livestream.ID)) {
			if p.state.CheckPriority("Kick", p.cfg) {
				slog.Info("stream found",
					p.prefix,
					slog.Int("id", stream.Livestream.ID),
				)
				if p.cfg.Plugins.Enabled {
					util.LuaCallReceiveFunction(l, fmt.Sprintf("%d", stream.Livestream.ID))
				}

				vod := &dggarchivermodel.VOD{
					Platform:    "kick",
					Downloader:  p.cfg.Platforms.Kick.Downloader,
					ID:          fmt.Sprintf("%d", stream.Livestream.ID),
					PlaybackURL: stream.URL,
					Title:       stream.Livestream.Title,
					StartTime:   time.Now().Format(time.RFC3339),
					EndTime:     "",
					Thumbnail:   strings.Split(strings.Split(stream.Livestream.Thumbnail.URL, ",")[0], " ")[0],
				}

				p.state.CurrentStreams.Kick = *vod

				bytes, err := json.Marshal(vod)
				if err != nil {
					slog.Error("unable to marshal vod",
						p.prefix,
						slog.String("id", vod.ID),
						slog.Any("err", err),
					)
					return nil
				}

				if err = p.cfg.NATS.NatsConnection.Publish(fmt.Sprintf("%s.job", p.cfg.NATS.Topic), bytes); err != nil {
					slog.Error("unable to publish message",
						p.prefix,
						slog.String("id", vod.ID),
						slog.Any("err", err),
					)
					return nil
				}

				if p.cfg.Plugins.Enabled {
					util.LuaCallSendFunction(l, vod)
				}
				p.state.SentVODs = append(p.state.SentVODs, fmt.Sprintf("kick:%s", vod.ID))
				p.state.Dump()
			} else {
				slog.Info("streaming on a different platform",
					p.prefix,
					slog.Int("id", stream.Livestream.ID),
				)
			}
		} else {
			slog.Info("already sent",
				p.prefix,
				slog.Int("id", stream.Livestream.ID),
			)
		}
	} else {
		p.state.CurrentStreams.Kick = dggarchivermodel.VOD{}
		slog.Info("not live",
			p.prefix,
		)
	}

	util.HealthCheck(p.cfg.Platforms.Kick.HealthCheck)

	return nil
}

func (p *Platform) scrape() *api {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://kick.com/api/v1/channels/%s", p.cfg.Platforms.Kick.Channel), nil)
	if err != nil {
		slog.Error("unable to create request",
			p.prefix,
			slog.Any("err", err),
		)
		os.Exit(1)
	}

	req.Header = http.Header{
		"accept":          {"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"},
		"accept-language": {"en-US,en;q=0.5"},
		"user-agent":      {"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/110.0.0.0 Safari/537.36"},
		http.HeaderOrderKey: {
			"accept",
			"accept-language",
			"user-agent",
		},
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		slog.Error("unable to make request",
			p.prefix,
			slog.Any("err", err),
		)
		return nil
	}

	defer resp.Body.Close()
	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("unable to read response",
			p.prefix,
			slog.Any("err", err),
		)
		return nil
	}

	var stream api
	err = json.Unmarshal(bytes, &stream)
	if err != nil {
		slog.Error("unable to unmarshal response",
			p.prefix,
			slog.Any("err", err),
		)
		return nil
	}

	return &stream
}
