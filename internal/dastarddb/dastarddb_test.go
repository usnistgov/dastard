package dastarddb

import (
	"context"
	"log"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2"
)

func TestConnection(t *testing.T) {
	auth := clickhouse.Auth{
		Database: "default",
		Username: "default",
		Password: "",
	}
	opt :=
		clickhouse.Options{
			Addr: []string{"localhost:9000"},
			Auth: auth,
		}
	conn, err := Connect(&opt)
	if err != nil {
		t.Error(err)
	}

	ctx := context.Background()
	// rows, err := conn.db.Query(ctx, "SELECT name, toString(uuid) as uuid_str FROM system.tables LIMIT 10")
	rows, err := conn.db.Query(ctx, "SHOW TABLES")
	if err != nil {
		log.Fatal(err)
	}

	for rows.Next() {
		var name, uuid string
		if err := rows.Scan(&name, &uuid); err != nil {
			log.Fatal(err)
		}
		log.Printf("name: %s, uuid: %s", name, uuid)
	}
}
