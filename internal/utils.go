package internal

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"
	"unsafe"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/storage"
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

func GetSpannerCurrentTime(client *spanner.Client) time.Time {
	in := &QuerySpannerSingleInput{
		Client: client,
		Query:  "select current_timestamp() as now",
	}

	rows, err := QuerySpannerSingle(context.Background(), in)
	if err != nil {
		return time.Time{}
	}

	type nowT struct {
		Now spanner.NullTime
	}

	for _, row := range rows {
		var v nowT
		err = row.ToStruct(&v)
		if err != nil {
			return time.Time{}
		}

		return v.Now.Time
	}

	return time.Time{}
}

// LockTimeDiff returns the difference between lock's start time + duration against
// Spanner's current time. Negative means already expired, else still active.
func LockTimeDiff(client *spanner.Client, start string, duration int64) (int64, error) {
	var q strings.Builder
	fmt.Fprintf(&q, "select timestamp_diff(timestamp_add(timestamp '%v', ", start)
	fmt.Fprintf(&q, "interval %v second), current_timestamp, second) as diff", duration)
	in := &QuerySpannerSingleInput{
		Client: client,
		Query:  q.String(),
	}

	rows, err := QuerySpannerSingle(context.Background(), in)
	if err != nil {
		return 0, err
	}

	type diffT struct {
		Diff spanner.NullInt64
	}

	for _, row := range rows {
		var v diffT
		err = row.ToStruct(&v)
		if err != nil {
			return 0, err
		}

		return v.Diff.Int64, nil
	}

	return 0, fmt.Errorf("Internal: something is wrong.")
}

func GetSnapshot(object string) ([]byte, error) {
	bucket := "alphaus-tokyo"
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("storage.NewClient: %w", err)
	}

	defer client.Close()
	ctx, cancel := context.WithTimeout(ctx, time.Second*50)
	defer cancel()

	rc, err := client.Bucket(bucket).Object(object).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("Object(%q).NewReader: %w", object, err)
	}

	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("io.ReadAll: %w", err)
	}

	return data, nil
}

func PutSnapshot(object string, data []byte) error {
	bucket := "alphaus-tokyo"
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("storage.NewClient: %w", err)
	}

	rdr := bytes.NewReader(data)
	ctx, cancel := context.WithTimeout(ctx, time.Second*50)
	defer cancel()

	o := client.Bucket(bucket).Object(object)

	// Optional: set a generation-match precondition to avoid potential race
	// conditions and data corruptions. The request to upload is aborted if the
	// object's generation number does not match your precondition.
	// For an object that does not yet exist, set the DoesNotExist precondition.
	o = o.If(storage.Conditions{DoesNotExist: true})
	// If the live object already exists in your bucket, set instead a
	// generation-match precondition using the live object's generation number.
	// attrs, err := o.Attrs(ctx)
	// if err != nil {
	// 	return fmt.Errorf("object.Attrs: %w", err)
	// }
	// o = o.If(storage.Conditions{GenerationMatch: attrs.Generation})

	// Upload an object with storage.Writer.
	wc := o.NewWriter(ctx)
	if _, err = io.Copy(wc, rdr); err != nil {
		return fmt.Errorf("io.Copy: %w", err)
	}

	if err := wc.Close(); err != nil {
		return fmt.Errorf("Writer.Close: %w", err)
	}

	return nil
}
