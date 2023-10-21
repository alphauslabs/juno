package internal

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/alphauslabs/juno/internal/flags"
	"github.com/golang/glog"
)

type SlackAttachment struct {
	// Fallback is our simple fallback text equivalent.
	Fallback string `json:"fallback"`

	// Color can be 'good', 'warning', 'danger', or any hex color code.
	Color string `json:"color,omitempty"`

	// Pretext is our text above the attachment section.
	Pretext string `json:"pretext,omitempty"`

	// Title is the notification title.
	Title string `json:"title,omitempty"`

	// TitleLink is the url link attached to the title.
	TitleLink string `json:"title_link,omitempty"`

	// Text is the main text in the attachment.
	Text string `json:"text"`

	// Footer is a brief text to help contextualize and identify an attachment.
	// Limited to 300 characters, and may be truncated further when displayed
	// to users in environments with limited screen real estate.
	Footer string `json:"footer,omitempty"`

	// Timestamp is an integer Unix timestamp that is used to related your attachment to
	// a specific time. The attachment will display the additional timestamp value as part
	// of the attachment's footer.
	Timestamp int64 `json:"ts,omitempty"`
}

type SlackNotify struct {
	Attachments []SlackAttachment `json:"attachments"`
}

func (sn *SlackNotify) SimpleNotify(slackUrl string) error {
	bp, err := json.Marshal(sn)
	if err != nil {
		return err
	}

	hc := http.DefaultClient
	r, err := http.NewRequest(http.MethodPost, slackUrl, bytes.NewBuffer(bp))
	if err != nil {
		return err
	}
	_, err = hc.Do(r)
	if err != nil {
		glog.Errorf("http.Do failed: %v", err)
		return err
	}

	return nil
}

// Optional 'args':
// [0] - color: good, warning, danger
// [1..] - channel(s) override
func TraceSlack(payload, title string, args ...string) {
	channel := *flags.Slack
	if channel == "" {
		return
	}

	t := title
	if t == "" {
		t = "juno error"
	}

	color := "danger"
	if len(args) > 0 {
		color = args[0]
	}

	slackPayload := SlackNotify{
		Attachments: []SlackAttachment{
			{
				Color:     color,
				Title:     t,
				Text:      payload,
				Footer:    "juno",
				Timestamp: time.Now().Unix(),
			},
		},
	}

	switch {
	case len(args) == 2: // one channel
		channel = args[1]
		slackPayload.SimpleNotify(channel)
	}
}
