package rumble

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	config "github.com/DggHQ/dggarchiver-config/notifier"
	dggarchivermodel "github.com/DggHQ/dggarchiver-model"
	"github.com/DggHQ/dggarchiver-notifier/platforms/implementation"
	"github.com/DggHQ/dggarchiver-notifier/state"
	"github.com/DggHQ/dggarchiver-notifier/util"
	"github.com/gocolly/colly/v2"
	lua "github.com/yuin/gopher-lua"
	"golang.org/x/exp/slices"
)

const (
	platformName   string = "rumble"
	platformMethod string = "scraper"
)

func init() {
	implementation.Map[fmt.Sprintf("%s_%s", platformName, platformMethod)] = New
}

type Platform struct {
	vodChan   chan *dggarchivermodel.VOD
	vodCheck  atomic.Bool
	c1        *colly.Collector
	c2        *colly.Collector
	cfg       *config.Config
	state     *state.State
	prefix    slog.Attr
	sleepTime time.Duration
}

type oembed struct {
	Title     string `json:"title"`
	Duration  int    `json:"duration"`
	Thumbnail string `json:"thumbnail_url"`
	HTML      string `json:"html"`
	EmbedID   string
}

// New returns a new Rumble platform struct
func New(cfg *config.Config, state *state.State) implementation.Platform {
	vodChan := make(chan *dggarchivermodel.VOD)

	c1 := colly.NewCollector()
	c1.DisableCookies()
	c1.AllowURLRevisit = true

	c2 := colly.NewCollector()
	c2.DisableCookies()
	c2.AllowURLRevisit = true

	p := Platform{
		vodChan:  vodChan,
		vodCheck: atomic.Bool{},
		c1:       c1,
		c2:       c2,
		cfg:      cfg,
		state:    state,
		prefix: slog.Group("platform",
			slog.String("name", platformName),
			slog.String("method", platformMethod),
		),
		sleepTime: time.Second * 60 * time.Duration(cfg.Platforms.Rumble.RefreshTime),
	}

	c1.OnHTML("a.video-item--a", func(h *colly.HTMLElement) {
		go func() {
			if !p.wasVODSent() {
				live := h.ChildAttr("span.video-item--live", "data-value")
				p.setVODSent()
				if len(live) != 0 {
					link := h.Attr("href")
					embedData := p.getOEmbed(link)
					vodChan <- &dggarchivermodel.VOD{
						Platform:    "rumble",
						Downloader:  cfg.Platforms.Rumble.Downloader,
						ID:          embedData.EmbedID,
						PlaybackURL: fmt.Sprintf("https://rumble.com%s", link),
						Title:       embedData.Title,
						StartTime:   time.Now().Format(time.RFC3339),
						EndTime:     "",
						Thumbnail:   embedData.Thumbnail,
					}
				} else {
					vodChan <- nil
				}
			}
		}()
	})

	c2.OnHTML("html", func(h *colly.HTMLElement) {
		go func() {
			if !p.wasVODSent() {
				liveDOM := h.DOM.Find(".watching-now")
				p.setVODSent()
				if len(liveDOM.Nodes) != 0 {
					linkDOM := h.DOM.Find("link[rel=canonical]")
					link, _ := linkDOM.Attr("href")
					embedData := p.getOEmbed(link)
					vodChan <- &dggarchivermodel.VOD{
						Platform:    "rumble",
						Downloader:  cfg.Platforms.Rumble.Downloader,
						ID:          embedData.EmbedID,
						PlaybackURL: link,
						Title:       embedData.Title,
						StartTime:   time.Now().Format(time.RFC3339),
						EndTime:     "",
						Thumbnail:   embedData.Thumbnail,
					}
				} else {
					vodChan <- nil
				}
			}
		}()
	})

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
	vod := p.scrape()

	if vod != nil {
		if !slices.Contains(p.state.SentVODs, fmt.Sprintf("rumble:%s", vod.ID)) {
			if p.state.CheckPriority("Rumble", p.cfg) {
				slog.Info("stream found",
					p.prefix,
					slog.String("id", vod.ID),
				)
				if p.cfg.Plugins.Enabled {
					util.LuaCallReceiveFunction(l, vod.ID)
				}

				p.state.CurrentStreams.Rumble = *vod

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
				p.state.SentVODs = append(p.state.SentVODs, fmt.Sprintf("rumble:%s", vod.ID))
				p.state.Dump()
			} else {
				slog.Info("streaming on a different platform",
					p.prefix,
					slog.String("id", vod.ID),
				)
			}
		} else {
			slog.Info("already sent",
				p.prefix,
				slog.String("id", vod.ID),
			)
		}
	} else {
		p.state.CurrentStreams.Rumble = dggarchivermodel.VOD{}
		slog.Info("not live",
			p.prefix,
		)
	}

	util.HealthCheck(p.cfg.Platforms.Rumble.HealthCheck)

	return nil
}

func (p *Platform) wasVODSent() bool {
	return p.vodCheck.Load()
}

func (p *Platform) setVODSent() {
	p.vodCheck.Store(true)
}

func (p *Platform) resetVODSent() {
	p.vodCheck.Store(false)
}

func (p *Platform) scrape() *dggarchivermodel.VOD {
	p.resetVODSent()

	if err := p.c1.Visit(fmt.Sprintf("https://rumble.com/c/%s?date=today", p.cfg.Platforms.Rumble.Channel)); err != nil {
		return nil
	}
	if err := p.c1.Visit(fmt.Sprintf("https://rumble.com/c/%s?date=this-week", p.cfg.Platforms.Rumble.Channel)); err != nil {
		return nil
	}
	if err := p.c1.Visit(fmt.Sprintf("https://rumble.com/c/%s?date=this-month", p.cfg.Platforms.Rumble.Channel)); err != nil {
		return nil
	}
	if err := p.c1.Visit(fmt.Sprintf("https://rumble.com/c/%s?date=this-year", p.cfg.Platforms.Rumble.Channel)); err != nil {
		return nil
	}
	if err := p.c1.Visit(fmt.Sprintf("https://rumble.com/c/%s", p.cfg.Platforms.Rumble.Channel)); err != nil {
		return nil
	}

	if err := p.c2.Visit(fmt.Sprintf("https://rumble.com/%s/live", p.cfg.Platforms.Rumble.Channel)); err != nil {
		return nil
	}

	return <-p.vodChan
}

func (p *Platform) getOEmbed(url string) *oembed {
	response, err := http.Get(fmt.Sprintf("https://rumble.com/api/Media/oembed.json/?url=%s", url))
	if err != nil {
		slog.Error("unable to get oembed data",
			p.prefix,
			slog.String("url", url),
			slog.Any("err", err),
		)
		return nil
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		slog.Error("unable to get oembed data",
			p.prefix,
			slog.String("url", url),
			slog.Int("status", response.StatusCode),
		)
		return nil
	}

	bytes, err := io.ReadAll(response.Body)
	if err != nil {
		slog.Error("unable to read response",
			p.prefix,
			slog.String("url", url),
			slog.Any("err", err),
		)
		return nil
	}

	data := oembed{}
	err = json.Unmarshal(bytes, &data)
	if err != nil {
		slog.Error("unable to unmarshal response",
			p.prefix,
			slog.String("url", url),
			slog.Any("err", err),
		)
		return nil
	}

	if data.HTML != "" {
		data.EmbedID = strings.SplitN(strings.SplitN(data.HTML, "https://rumble.com/embed/", 2)[1], "/", 2)[0]
	}

	return &data
}
