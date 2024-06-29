package notifications

import (
	"bytes"
	"text/template"

	dggarchivermodel "github.com/DggHQ/dggarchiver-model"
)

const (
	receive string = `
		Platform: {{ .Platform }}
		ID: {{ .ID }}
	`
	send string = `
		Platform: {{ .Platform }}
		ID: {{ .ID }}
	`
)

var (
	receiveTemplate, _ = template.New("receive").Parse(receive)
	sendTemplate, _    = template.New("send").Parse(send)
)

type n struct {
	Platform string
	ID       string
}

func GetReceiveMessage(platform, id string) string {
	var b bytes.Buffer

	_ = receiveTemplate.Execute(&b, n{
		Platform: platform,
		ID:       id,
	})

	return b.String()
}

func GetSendMessage(vod *dggarchivermodel.VOD) string {
	var b bytes.Buffer

	_ = sendTemplate.Execute(&b, vod)

	return b.String()
}