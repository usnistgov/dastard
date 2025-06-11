// Package dastarddb provides classes that read or write to a ClickHouse database.
package dastarddb

import (
	"context"
	"fmt"

	"github.com/ClickHouse/clickhouse-go/v2"
)

type DastardDBConnection struct {
	connection clickhouse.Conn
}

func Connect(opt *clickhouse.Options) (c *DastardDBConnection, err error) {
	client := clickhouse.ClientInfo{
		Products: []struct {
			Name    string
			Version string
		}{
			{Name: "an-example-go-client", Version: "0.1"},
		},
	}
	opt.ClientInfo = client
	opt.TLS = nil
	ctx := context.Background()
	conn, err := clickhouse.Open(opt)
	if err != nil {
		return nil, err
	}
	if err = conn.Ping(ctx); err != nil {
		if exception, ok := err.(*clickhouse.Exception); ok {
			fmt.Printf("Exception [%d] %s \n%s\n", exception.Code, exception.Message, exception.StackTrace)
		}
		return nil, err
	}
	c = &DastardDBConnection{connection: conn}
	return c, nil
}
