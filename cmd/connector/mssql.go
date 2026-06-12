package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"runtime/debug"
	"time"

	"github.com/cin-yi-wei/dataseai-connector/pkg/protocol"
	mssql "github.com/microsoft/go-mssqldb"
	"github.com/microsoft/go-mssqldb/msdsn"
	"golang.org/x/crypto/ssh"
)

// SQLServerExecutor runs a query against a Microsoft SQL Server. Like the
// other executors it opens a fresh connection per request and can tunnel the
// connection through SSH when the request supplies SSH config.
type SQLServerExecutor struct {
	DialTimeout time.Duration
}

func (e SQLServerExecutor) Run(ctx context.Context, req protocol.QueryRequest, sink Sink) (err error) {
	// go-mssqldb can panic on some drivers/server combinations; recover so a
	// single bad query can't crash the whole connector process (which would
	// otherwise just drop the session with no diagnostic).
	defer func() {
		if r := recover(); r != nil {
			log.Printf("mssql: PANIC handling query: %v\n%s", r, debug.Stack())
			err = fmt.Errorf("mssql panic: %v", r)
		}
	}()

	dialTimeout := e.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = 10 * time.Second
	}

	db, cleanup, err := e.openDB(req.Target, dialTimeout)
	if err != nil {
		return err
	}
	defer cleanup()
	defer db.Close()
	db.SetMaxOpenConns(1)

	// Establish the connection up front with a bounded timeout so a stalled
	// TLS handshake or login surfaces as a logged error instead of hanging
	// until the caller's deadline (which leaves no diagnostic behind).
	pingCtx, cancelPing := context.WithTimeout(ctx, 12*time.Second)
	connectStart := time.Now()
	if perr := db.PingContext(pingCtx); perr != nil {
		cancelPing()
		log.Printf("mssql: connect to %s:%d failed after %s: %v",
			req.Target.Host, req.Target.Port, time.Since(connectStart).Round(time.Millisecond), perr)
		return fmt.Errorf("connect: %w", perr)
	}
	cancelPing()
	log.Printf("mssql: connected to %s:%d in %s",
		req.Target.Host, req.Target.Port, time.Since(connectStart).Round(time.Millisecond))

	rows, err := db.QueryContext(ctx, req.SQL)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return fmt.Errorf("column types: %w", err)
	}
	meta := make([]protocol.ColInfo, len(colTypes))
	for i, c := range colTypes {
		meta[i] = protocol.ColInfo{Name: c.Name(), Type: c.DatabaseTypeName()}
	}
	if err := sink.Meta(meta); err != nil {
		return err
	}

	batchSize := req.BatchSize
	if batchSize <= 0 {
		batchSize = 200
	}
	nCols := len(colTypes)
	rowVals := make([]any, nCols)
	rowPtrs := make([]any, nCols)
	for i := range rowVals {
		rowPtrs[i] = &rowVals[i]
	}

	batch := make([][]any, 0, batchSize)
	rowCount := 0
	for rows.Next() {
		if req.MaxRows > 0 && rowCount >= req.MaxRows {
			break
		}
		if err := rows.Scan(rowPtrs...); err != nil {
			return fmt.Errorf("scan: %w", err)
		}
		out := make([]any, nCols)
		for i, v := range rowVals {
			out[i] = jsonable(v)
		}
		batch = append(batch, out)
		rowCount++

		if len(batch) >= batchSize {
			if err := sink.Rows(batch); err != nil {
				return err
			}
			batch = batch[:0]
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iter: %w", err)
	}
	if len(batch) > 0 {
		if err := sink.Rows(batch); err != nil {
			return err
		}
	}
	return nil
}

func (e SQLServerExecutor) openDB(t protocol.MySQLTarget, dialTimeout time.Duration) (*sql.DB, func(), error) {
	host := t.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := t.Port
	if port == 0 {
		port = 1433
	}

	cfg := msdsn.Config{
		Host:     host,
		Port:     uint64(port),
		User:     t.User,
		Password: t.Password,
		Database: t.Database,
		// Always encrypt the connection (encrypt=true). SQL Server instances with
		// Force Encryption enabled drop unencrypted or login-only-encrypted
		// connections, which manifests as a hang until the caller's deadline.
		// Full encryption matches what `sqlcmd -C` does and works whether or not
		// the server forces it. TrustServerCertificate accepts the (typically
		// self-signed) certificate so the TLS handshake completes without a CA.
		Encryption:             msdsn.EncryptionRequired,
		TrustServerCertificate: true,
		DialTimeout:            dialTimeout,
	}
	connector := mssql.NewConnectorConfig(cfg)

	cleanup := func() {}
	if !t.SSH.IsZero() {
		sshClient, err := openSSHClient(*t.SSH, dialTimeout)
		if err != nil {
			return nil, nil, err
		}
		cleanup = func() { sshClient.Close() }
		connector.Dialer = mssqlSSHDialer{client: sshClient}
	}

	return sql.OpenDB(connector), cleanup, nil
}

// mssqlSSHDialer implements the go-mssqldb Dialer interface, routing the
// TCP connection through an established SSH tunnel.
type mssqlSSHDialer struct {
	client *ssh.Client
}

func (d mssqlSSHDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		conn, err := d.client.Dial("tcp", addr)
		ch <- result{conn, err}
	}()
	select {
	case r := <-ch:
		return r.conn, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
