package implementation

import (
	"log/slog"
	"time"

	config "github.com/DggHQ/dggarchiver-config/notifier"
	"github.com/DggHQ/dggarchiver-notifier/state"
)

type newPlatformFunc func(*config.Config, *state.State) Platform

var Map = map[string]newPlatformFunc{}

type Platform interface {
	CheckLivestream() error
	GetPrefix() slog.Attr
	GetSleepTime() time.Duration
}

func LaunchLoop(imp Platform) {
	prefix := imp.GetPrefix()
	sleep := imp.GetSleepTime()

	go func() {
		timeout := 0

		for {
			if timeout > 0 {
				slog.Info("sleeping before starting",
					prefix,
					slog.Int("duration", timeout),
				)
				time.Sleep(time.Second * time.Duration(timeout))
			}
			err := imp.CheckLivestream()
			if err != nil {
				slog.Error("error occurred while checking, restarting the loop",
					prefix,
					slog.Any("err", err),
				)
				switch {
				case timeout == 0:
					timeout = 1
				case (timeout >= 1 && timeout <= 32):
					timeout *= 2
				}
				continue
			}
			timeout = 0
			slog.Debug("sleeping",
				prefix,
				slog.Int("duration", int(sleep.Minutes())),
			)
			time.Sleep(sleep)
		}
	}()
}
