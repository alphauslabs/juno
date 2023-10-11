package internal

import (
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
)

// newEvent returns a JSON (by default) standard cloudevent.
// Optional arguments:
// args[0] = source (string)
// args[1] = type (string)
// args[2] = contenttype (string), default: application/json
// See https://github.com/cloudevents/sdk-go for more details.
func NewEvent(data interface{}, args ...string) cloudevents.Event {
	src := "ouchan/internal"
	typ := "ouchan.event.internal"
	ctt := cloudevents.ApplicationJSON
	switch {
	case len(args) >= 3:
		src = args[0]
		typ = args[1]
		ctt = args[2]
	case len(args) == 2:
		src = args[0]
		typ = args[1]
	case len(args) == 1:
		src = args[0]
	}

	event := cloudevents.NewEvent()
	event.SetID(uuid.New().String())
	event.SetSource(src)
	event.SetType(typ)
	event.SetData(ctt, data)
	return event
}
