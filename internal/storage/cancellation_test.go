package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
)

// A controlled driver models database/sql's cancelled-transaction error without
// relying on the goroutine scheduling race between automatic rollback/Commit.
type cancelledConnector struct{ cancel context.CancelFunc }

func (c cancelledConnector) Connect(context.Context) (driver.Conn, error) {
	return cancelledConn(c), nil
}
func (c cancelledConnector) Driver() driver.Driver { return cancelledDriver(c) }

type cancelledDriver struct{ cancel context.CancelFunc }

func (c cancelledDriver) Open(string) (driver.Conn, error) { return cancelledConn(c), nil }

type cancelledConn struct{ cancel context.CancelFunc }

func (cancelledConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("fixture does not prepare statements")
}
func (cancelledConn) Close() error              { return nil }
func (cancelledConn) Begin() (driver.Tx, error) { return cancelledTransaction{}, nil }
func (c cancelledConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	c.cancel()
	return nil, sql.ErrTxDone
}

type cancelledTransaction struct{}

func (cancelledTransaction) Commit() error   { return sql.ErrTxDone }
func (cancelledTransaction) Rollback() error { return nil }
func TestCancelledTransactionPreservesCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db := sql.OpenDB(cancelledConnector{cancel})
	defer func() { _ = db.Close() }()
	s := &Store{db: db}
	err := s.SaveBatch(ctx, []Job{{ID: "cancelled", Version: 1, State: "downloading"}})
	if !errors.Is(err, context.Canceled) || !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("lost cancellation or database error: %v", err)
	}
}
