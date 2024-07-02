package main

import (
	"log/slog"

	config "github.com/DggHQ/dggarchiver-config/notifier"
	"github.com/DggHQ/dggarchiver-notifier/platforms"
	"github.com/DggHQ/dggarchiver-notifier/state"
	"github.com/DggHQ/dggarchiver-notifier/util"

	_ "github.com/DggHQ/dggarchiver-notifier/platforms/kick"
	_ "github.com/DggHQ/dggarchiver-notifier/platforms/rumble"
	_ "github.com/DggHQ/dggarchiver-notifier/platforms/yt"
)

func main() {
	cfg := config.New()

	state := state.New(cfg)
	state.Load()

	enabledPlatforms := util.GetEnabledPlatforms(cfg)
	slog.Info("running the notifier service", slog.Any("platforms", enabledPlatforms))

	p := platforms.New(cfg, &state, enabledPlatforms)
	p.Start()
}
