package notifications

import (
	"bytes"
	"strings"
	"text/template"

	dggarchivermodel "github.com/DggHQ/dggarchiver-model"
)

var (
	receive = strings.Join([]string{
		"Platform: {{ .Platform }}",
		"ID: {{ .VID }}",
	}, "\n")

	send = strings.Join([]string{
		"Platform: {{ .Platform }}",
		"ID: {{ .VID }}",
	}, "\n")
)

var (
	receiveTemplate, _ = template.New("receive").Parse(receive)
	sendTemplate, _    = template.New("send").Parse(send)
)

type n struct {
	Platform string
	VID      string
}

func GetReceiveMessage(platform, id string) string {
	var b bytes.Buffer

	_ = receiveTemplate.Execute(&b, n{
		Platform: platform,
		VID:      id,
	})

	return b.String()
}

func GetSendMessage(vod *dggarchivermodel.VOD) string {
	var b bytes.Buffer

	_ = sendTemplate.Execute(&b, vod)

	return b.String()
}
