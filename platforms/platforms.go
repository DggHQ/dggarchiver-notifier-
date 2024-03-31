package platforms

import (
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"time"

	config "github.com/DggHQ/dggarchiver-config/notifier"
	"github.com/DggHQ/dggarchiver-notifier/platforms/implementation"
	"github.com/DggHQ/dggarchiver-notifier/state"
)

type Platforms struct {
	enabledPlatforms []string
	wg               sync.WaitGroup
	state            *state.State
	cfg              *config.Config
}

func New(cfg *config.Config, state *state.State) *Platforms {
	var (
		enabledPlatforms       = []string{}
		enabledPlatformsSorted = []string{}
		count, i               int64
	)

	platformsValue := reflect.ValueOf(cfg.Platforms)
	platformsFields := reflect.VisibleFields(reflect.TypeOf(cfg.Platforms))
	for _, field := range platformsFields {
		if platformsValue.FieldByName(field.Name).FieldByName("Enabled").Bool() {
			enabledPlatforms = append(enabledPlatforms, field.Name)
			count++
		}
	}

	for i = 1; i < count+1; i++ {
		for _, field := range enabledPlatforms {
			if platformsValue.FieldByName(field).FieldByName("Priority").Int() == i {
				name := strings.ToLower(field)
				method := strings.ToLower(platformsValue.FieldByName(field).FieldByName("Method").String())
				enabledPlatformsSorted = append(enabledPlatformsSorted, fmt.Sprintf("%s_%s", name, method))
			}
		}
	}

	if len(enabledPlatformsSorted) == 0 {
		for _, field := range platformsFields {
			if platformsValue.FieldByName(field.Name).FieldByName("Enabled").Bool() {
				name := strings.ToLower(field.Name)
				method := strings.ToLower(platformsValue.FieldByName(field.Name).FieldByName("Method").String())
				enabledPlatformsSorted = append(enabledPlatformsSorted, fmt.Sprintf("%s_%s", name, method))
			}
		}
	}

	return &Platforms{
		enabledPlatforms: enabledPlatformsSorted,
		wg:               sync.WaitGroup{},
		state:            state,
		cfg:              cfg,
	}
}

func (p *Platforms) Start() {
	for _, v := range p.enabledPlatforms {
		imp := implementation.Map[v](p.cfg, p.state)
		slog.Info("running platform loop",
			imp.GetPrefix(),
			slog.Int("refresh_time", p.cfg.Platforms.YouTube.RefreshTime),
		)
		p.wg.Add(1)
		implementation.LaunchLoop(p.cfg, imp)

		time.Sleep(time.Second * 1)
	}

	p.wg.Wait()
}
