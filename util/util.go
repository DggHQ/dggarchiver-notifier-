package util

import (
	"log/slog"
	"net/http"
	"reflect"
	"time"

	config "github.com/DggHQ/dggarchiver-config/notifier"
)

var healthCheckClient = &http.Client{
	Timeout: 10 * time.Second,
}

func HealthCheck(url string) {
	if url == "" {
		return
	}

	_, err := healthCheckClient.Head(url)
	if err != nil {
		slog.Error("unable to send healthcheck request", slog.Any("err", err))
	}
}

func GetEnabledPlatforms(cfg *config.Config) []string {
	enabledPlatforms := []string{}

	platformsValue := reflect.ValueOf(cfg.Platforms)
	platformsFields := reflect.VisibleFields(reflect.TypeOf(cfg.Platforms))
	for _, field := range platformsFields {
		if platformsValue.FieldByName(field.Name).FieldByName("Enabled").Bool() {
			enabledPlatforms = append(enabledPlatforms, field.Name)
		}
	}

	return enabledPlatforms
}
