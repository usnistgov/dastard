//go:build nodb
// +build nodb

// This drop-in replacement for the dastard/internal/mysql package can be selected at
// build time with the "-tags nodb" build tag (or by using the Makefile as
// "NODB=true make build"). It puts no-ops in place of the usual communication with a
// MySQL server.
//
// TODO: This entire conditional compilation might be pointless, because it seems
// that packages database/sql and github.com/go-sql-driver/mysql have no library
// dependencies. They just use the regular net package. In that case, compiling in the
// regular mysql.go file might have almost zero impact on the dependencies.
// Consider removing (8 March 2023, JWF)?

package mysql

import "fmt"

func StartMySQLConnection(abort <-chan struct{}) {}
func RecordDatarun(msg *DatarunMessage)          {}
func RecordDatafile(msg *DatafileMessage)        {}

func PingMySQLServer() bool {
	fmt.Println("Ping to MySQL server not tried: Dastard was built with the '-tags nodb' flag.")
	return false
}
