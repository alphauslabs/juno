package internal

import (
	"context"
	"fmt"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

type QuerySpannerSingleInput struct {
	Client   *spanner.Client
	Database string
	Query    string
	Params   map[string]interface{} // if provided, assumed that Query has params
}

// QuerySpannerSingle is a helper function that does simple Spanner db queries. You can override the client object.
// It will use the Single() transaction from the client by default. If no option for auth is provided, it will
// authenticate using the GOOGLE_APPLICATION_CREDENTIALS environment variable.
//
// When a client is provided, it will use the client without calling Close() and ignores the opts array.
// Otherwise, it will create a local client from the 'Database' string together with the opts array.
func QuerySpannerSingle(ctx context.Context, in *QuerySpannerSingleInput, opts ...option.ClientOption) ([]*spanner.Row, error) {
	if in == nil {
		return nil, fmt.Errorf("input is nil")
	}

	var local bool
	var err error
	client := in.Client
	if client == nil {
		local = true
		client, err = spanner.NewClient(ctx, in.Database, opts...)
		if err != nil {
			return nil, err
		}
	}

	if local {
		defer client.Close()
	}

	ret := []*spanner.Row{}
	q := spanner.Statement{SQL: in.Query}
	if in.Params != nil {
		q = spanner.Statement{SQL: in.Query, Params: in.Params}
	}

	iter := client.Single().Query(ctx, q)
	defer iter.Stop()

	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}

		if err != nil {
			return nil, err
		}

		ret = append(ret, row)
	}

	return ret, nil
}
