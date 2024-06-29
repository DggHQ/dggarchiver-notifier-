package yt

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	config "github.com/DggHQ/dggarchiver-config/notifier"
	dggarchivermodel "github.com/DggHQ/dggarchiver-model"
	"github.com/DggHQ/dggarchiver-notifier/platforms/implementation"
	"github.com/DggHQ/dggarchiver-notifier/state"
	"github.com/DggHQ/dggarchiver-notifier/util"
	lua "github.com/yuin/gopher-lua"
	"golang.org/x/exp/slices"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/youtube/v3"
)

type API struct {
	cfg       *config.Config
	state     *state.State
	prefix    slog.Attr
	sleepTime time.Duration
}

// New returns a new YouTube API platform struct
func NewAPI(cfg *config.Config, state *state.State) implementation.Platform {
	p := API{
		cfg:   cfg,
		state: state,
		prefix: slog.Group("platform",
			slog.String("name", platformName),
			slog.String("method", apiMethod),
		),
		sleepTime: time.Second * 60 * time.Duration(cfg.Platforms.YouTube.RefreshTime),
	}

	return &p
}

// GetPrefix returns a slog.Attr group for platform p
func (p *API) GetPrefix() slog.Attr {
	return p.prefix
}

// GetSleepTime returns sleep duration for platform p
func (p *API) GetSleepTime() time.Duration {
	return p.sleepTime
}

// CheckLivestream checks for an existing livestream on platform p,
// and, if found, publishes the info to NATS
func (p *API) CheckLivestream(l *lua.LState) error {
	vid, etagEnd, err := p.getLivestreamID(p.state.SearchETag)
	if err != nil {
		if googleapi.IsNotModified(err) {
			slog.Info("identical etag, skipping",
				p.prefix,
				slog.String("etag", etagEnd),
			)
			return nil
		}
		return err
	}

	p.state.SearchETag = etagEnd
	p.state.Dump()

	if len(vid) > 0 {
		if !slices.Contains(p.state.SentVODs, fmt.Sprintf("youtube:%s", vid[0].Id)) {
			if p.state.CheckPriority("YouTube", p.cfg) {
				slog.Info("stream found",
					p.prefix,
					slog.String("id", vid[0].Id),
				)
				if p.cfg.Plugins.Enabled {
					util.LuaCallReceiveFunction(l, vid[0].Id)
				}
				vod := &dggarchivermodel.VOD{
					Platform:  "youtube",
					ID:        vid[0].Id,
					PubTime:   vid[0].Snippet.PublishedAt,
					Title:     vid[0].Snippet.Title,
					StartTime: vid[0].LiveStreamingDetails.ActualStartTime,
					EndTime:   vid[0].LiveStreamingDetails.ActualEndTime,
					Thumbnail: vid[0].Snippet.Thumbnails.Medium.Url,
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
					slog.String("id", vid[0].Id),
				)
			}
		} else {
			slog.Info("already sent",
				p.prefix,
				slog.String("id", vid[0].Id),
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

func (p *API) getLivestreamID(etag string) ([]*youtube.Video, string, error) {
	resp, err := p.cfg.Platforms.YouTube.Service.Search.List([]string{"snippet"}).IfNoneMatch(etag).EventType("live").ChannelId(p.cfg.Platforms.YouTube.Channel).Type("video").Do()
	if err != nil {
		return nil, etag, err
	}

	if len(resp.Items) > 0 {
		id, _, err := p.getVideoInfo(resp.Items[0].Id.VideoId, "")
		if err != nil && !googleapi.IsNotModified(err) {
			return id, resp.Etag, nil
		}
		return id, resp.Etag, nil
	}

	return nil, resp.Etag, nil
}

func (p *API) getVideoInfo(id string, etag string) ([]*youtube.Video, string, error) {
	resp, err := p.cfg.Platforms.YouTube.Service.Videos.List([]string{"snippet", "liveStreamingDetails"}).IfNoneMatch(etag).Id(id).Do()
	if err != nil {
		return nil, etag, err
	}

	return resp.Items, resp.Etag, nil
}
