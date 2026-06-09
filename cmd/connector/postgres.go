package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"time"

	"github.com/cin-yi-wei/dataseai-connector/pkg/protocol"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/ssh"
)

// PostgreSQLExecutor runs a query against a PostgreSQL server. Connection
// details come from the request; a fresh connection is opened per query.
type PostgreSQLExecutor struct {
	DialTimeout time.Duration
}

func (e PostgreSQLExecutor) Run(ctx context.Context, req protocol.QueryRequest, sink Sink) error {
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

func (e PostgreSQLExecutor) openDB(t protocol.MySQLTarget, dialTimeout time.Duration) (*sql.DB, func(), error) {
	host := t.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := t.Port
	if port == 0 {
		port = 5432
	}

	cfg, err := pgx.ParseConfig(fmt.Sprintf(
		"host=%s port=%d sslmode=disable", host, port,
	))
	if err != nil {
		return nil, nil, fmt.Errorf("pg config: %w", err)
	}
	cfg.User = t.User
	cfg.Password = t.Password
	if t.Database != "" {
		cfg.Database = t.Database
	}

	var sshClient *ssh.Client
	cleanup := func() {}
	if !t.SSH.IsZero() {
		sshClient, err = openSSHClient(*t.SSH, dialTimeout)
		if err != nil {
			return nil, nil, err
		}
		cleanup = func() { sshClient.Close() }
		cfg.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) {
			type result struct {
				conn net.Conn
				err  error
			}
			ch := make(chan result, 1)
			go func() {
				conn, err := sshClient.Dial(network, addr)
				ch <- result{conn, err}
			}()
			select {
			case r := <-ch:
				return r.conn, r.err
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	db := stdlib.OpenDB(*cfg)
	return db, cleanup, nil
}
