package util

import (
	"log/slog"
	"net/http"
	"time"
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
