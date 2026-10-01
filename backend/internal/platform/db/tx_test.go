package db

import (
	"errors"
	"fmt"
	"io"
	"net"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

func TestUnreachable(t *testing.T) {
	for _, err := range []error{
		&pgconn.PgError{Code: "57P01"}, // terminating connection due to administrator command
		&pgconn.PgError{Code: "57P03"}, // the database system is starting up
		&pgconn.PgError{Code: "08006"}, // connection failure
		fmt.Errorf("query: %w", &net.OpError{Op: "dial", Err: errors.New("connection refused")}),
		io.ErrUnexpectedEOF,
	} {
		assert.True(t, Unreachable(err), "%v", err)
	}
	for _, err := range []error{
		&pgconn.PgError{Code: "23505"}, // unique violation: the database answered
		&pgconn.PgError{Code: "40001"}, // serialization failure
		errors.New("no rows"),
	} {
		assert.False(t, Unreachable(err), "%v", err)
	}
}
