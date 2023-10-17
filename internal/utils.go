package internal

import (
	"unsafe"

	"cloud.google.com/go/spanner"
)

var (
	SpannerString = func(s spanner.NullString) string {
		switch {
		case !s.IsNull():
			return s.String()
		default:
			return ""
		}
	}
)

func StringToBytes(s string) []byte {
	return *(*[]byte)(unsafe.Pointer(
		&struct {
			string
			int
		}{s, len(s)},
	))
}
