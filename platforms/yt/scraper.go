package yt

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
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

var (
	ErrUnableToParseInfo = errors.New("unable to parse youtube video page data")
	ytRegexp             = regexp.MustCompile(`\/watch\?v=([^\"]*)`)
)

type videoSchemaMicrodata struct {
	Invalid   bool
	Title     string
	PubTime   string
	StartTime string
	EndTime   string
	Thumbnail string
}

type Scraper struct {
	c            *colly.Collector
	c2           *colly.Collector
	index        int
	idChan       chan string
	idDoneChan   chan struct{}
	infoChan     chan videoSchemaMicrodata
	infoDoneChan chan struct{}
	cfg          *config.Config
	state        *state.State
	prefix       slog.Attr
	sleepTime    time.Duration
}

// New returns a new YouTube Scraper platform struct
func NewScraper(cfg *config.Config, state *state.State) implementation.Platform {
	c := colly.NewCollector()
	// disable cookie handling to bypass youtube consent screen
	c.DisableCookies()
	c.AllowURLRevisit = true

	c2 := colly.NewCollector()
	c2.DisableCookies()
	c2.AllowURLRevisit = true

	idChan := make(chan string)
	idDoneChan := make(chan struct{})
	infoChan := make(chan videoSchemaMicrodata)
	infoDoneChan := make(chan struct{})

	p := Scraper{
		c:            c,
		c2:           c2,
		idChan:       idChan,
		idDoneChan:   idDoneChan,
		infoChan:     infoChan,
		infoDoneChan: infoDoneChan,
		cfg:          cfg,
		state:        state,
		prefix: slog.Group("platform",
			slog.String("name", platformName),
			slog.String("method", scraperMethod),
		),
		sleepTime: time.Second * 60 * time.Duration(cfg.Platforms.YouTube.RefreshTime),
	}

	c.OnResponse(func(r *colly.Response) {
		p.index = strings.Index(string(r.Body), "Started streaming ")
	})

	c.OnHTML("link[href][rel='canonical']", func(h *colly.HTMLElement) {
		go func() {
			if p.index != -1 {
				idChan <- ytRegexp.FindStringSubmatch(h.Attr("href"))[1]
			} else {
				idChan <- ""
			}
		}()
	})

	c.OnScraped(func(_ *colly.Response) {
		go func() {
			p.idDoneChan <- struct{}{}
		}()
	})

	c2.OnHTML("div[itemscope]", func(h *colly.HTMLElement) {
		go func() {
			info := videoSchemaMicrodata{}

			h.ForEachWithBreak("[itemprop]", func(_ int, h *colly.HTMLElement) bool {
				prop := h.Attr("itemprop")
				content := h.Attr("content")
				if content == "" {
					content = h.Attr("href")
				}

				switch prop {
				case "name":
					if content == "" {
						info.Invalid = true
						return false
					}
					if h.Name == "meta" {
						info.Title = content
					}
				case "datePublished":
					if content == "" {
						info.Invalid = true
						return false
					}
					if h.Name == "meta" {
						pubTimeParsed, err := time.Parse("2006-01-02T15:04:05-07:00", content)
						if err != nil {
							info.Invalid = true
							return false
						}
						info.PubTime = pubTimeParsed.UTC().Format(time.RFC3339)
					}
				case "startDate":
					if content == "" {
						info.Invalid = true
						return false
					}
					if h.Name == "meta" {
						startTimeParsed, err := time.Parse("2006-01-02T15:04:05-07:00", content)
						if err != nil {
							info.Invalid = true
							return false
						}
						info.StartTime = startTimeParsed.UTC().Format(time.RFC3339)
					}
				case "endDate":
					if content == "" {
						info.Invalid = true
						return false
					}
					if h.Name == "meta" {
						endTimeParsed, err := time.Parse("2006-01-02T15:04:05-07:00", content)
						if err != nil {
							info.Invalid = true
							return false
						}
						info.EndTime = endTimeParsed.UTC().Format(time.RFC3339)
					}
				case "thumbnailUrl":
					if content == "" {
						info.Invalid = true
						return false
					}
					if h.Name == "link" {
						info.Thumbnail = content
					}
				}

				return true
			})

			p.infoChan <- info
		}()
	})

	c2.OnScraped(func(_ *colly.Response) {
		go func() {
			p.infoDoneChan <- struct{}{}
		}()
	})

	return &p
}

// GetPrefix returns a slog.Attr group for platform p
func (p *Scraper) GetPrefix() slog.Attr {
	return p.prefix
}

// GetSleepTime returns sleep duration for platform p
func (p *Scraper) GetSleepTime() time.Duration {
	return p.sleepTime
}

// CheckLivestream checks for an existing livestream on platform p,
// and, if found, publishes the info to NATS
func (p *Scraper) CheckLivestream(l *lua.LState) error {
	id := p.scrape()

	if id != "" {
		if !slices.Contains(p.state.SentVODs, fmt.Sprintf("youtube:%s", id)) {
			if p.state.CheckPriority("YouTube", p.cfg) {
				vid, err := p.getVideoInfo(id)
				if err != nil {
					return err
				}
				if vid == nil {
					slog.Warn("no info found",
						p.prefix,
						slog.String("id", id),
					)
					return nil
				}

				slog.Info("stream found",
					p.prefix,
					slog.String("id", id),
				)
				if p.cfg.Plugins.Enabled {
					util.LuaCallReceiveFunction(l, id)
				}

				vod := &dggarchivermodel.VOD{
					Platform:   "youtube",
					Downloader: p.cfg.Platforms.YouTube.Downloader,
					ID:         id,
					PubTime:    vid.PubTime,
					Title:      vid.Title,
					StartTime:  vid.StartTime,
					EndTime:    vid.EndTime,
					Thumbnail:  vid.Thumbnail,
				}

				p.state.CurrentStreams.YouTube = *vod

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
				p.state.SentVODs = append(p.state.SentVODs, fmt.Sprintf("youtube:%s", vod.ID))
				p.state.Dump()
			} else {
				slog.Info("streaming on a different platform",
					p.prefix,
					slog.String("id", id),
				)
			}
		} else {
			slog.Info("already sent",
				p.prefix,
				slog.String("id", id),
			)
		}
	} else {
		p.state.CurrentStreams.YouTube = dggarchivermodel.VOD{}
		slog.Info("not live",
			p.prefix,
		)
	}

	util.HealthCheck(p.cfg.Platforms.YouTube.HealthCheck)

	return nil
}

func (p *Scraper) scrape() string {
	if err := p.c.Visit(fmt.Sprintf("https://youtube.com/channel/%s/live?hl=en", p.cfg.Platforms.YouTube.Channel)); err != nil {
		return ""
	}

	select {
	case <-p.idDoneChan:
		return ""
	case id := <-p.idChan:
		<-p.idDoneChan
		return id
	}
}

func (p *Scraper) getVideoInfo(id string) (*videoSchemaMicrodata, error) {
	if err := p.c2.Visit(fmt.Sprintf("https://youtube.com/watch?v=%s", id)); err != nil {
		return nil, err
	}

	select {
	case <-p.infoDoneChan:
		return nil, nil
	case data := <-p.infoChan:
		if data.Invalid {
			return nil, ErrUnableToParseInfo
		}
		<-p.infoDoneChan
		return &data, nil
	}
}
