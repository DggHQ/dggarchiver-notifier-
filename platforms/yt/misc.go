package yt

import (
	"fmt"

	"github.com/DggHQ/dggarchiver-notifier/platforms/implementation"
)

const (
	platformName  string = "youtube"
	scraperMethod string = "scraper"
	apiMethod     string = "api"
)

func init() {
	implementation.Map[fmt.Sprintf("%s_%s", platformName, scraperMethod)] = NewScraper
	implementation.Map[fmt.Sprintf("%s_%s", platformName, apiMethod)] = NewAPI
}
