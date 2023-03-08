//go:build nodb
// +build nodb

// This drop-in replacement for the dastard/internal/mysql package can be selected at
// build time with the "-tags nodb" build tag (or by using the Makefile as
// "NODB=true make build"). It puts no-ops in place of the usual communication with a
// MySQL server.

package mysql

import "fmt"

func StartMySQLConnection(abort <-chan struct{}) {}
func RecordDatarun(msg *DatarunMessage)          {}
func RecordDatafile(msg *DatafileMessage)        {}

func PingMySQLServer() bool {
	fmt.Println("Ping to MySQL server not tried: Dastard was built with the '-tags nodb' flag.")
	return false
}
