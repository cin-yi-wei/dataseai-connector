package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"time"

	_ "github.com/bytehouse-cloud/driver-go/sql"
	"github.com/cin-yi-wei/dataseai-connector/pkg/protocol"
)

// ByteHouseExecutor runs queries against a ByteHouse server (ClickHouse-
// compatible) using the bytehouse TCP driver. A fresh connection is opened per
// request; no pooling.
type ByteHouseExecutor struct {
	DialTimeout time.Duration
}

func (e ByteHouseExecutor) Run(ctx context.Context, req protocol.QueryRequest, sink Sink) error {
	dsn := buildByteHouseDSN(req.Target, e.DialTimeout)
	db, err := sql.Open("bytehouse", dsn)
	if err != nil {
		return fmt.Errorf("bytehouse open: %w", err)
	}
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
		return sink.Rows(batch)
	}
	return nil
}

func buildByteHouseDSN(t protocol.MySQLTarget, dialTimeout time.Duration) string {
	if dialTimeout <= 0 {
		dialTimeout = 10 * time.Second
	}
	host := t.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := t.Port
	if port == 0 {
		port = 19000
	}
	u := &url.URL{
		Scheme: "tcp",
		Host:   fmt.Sprintf("%s:%d", host, port),
	}
	q := url.Values{}
	if t.User != "" {
		q.Set("user", t.User)
	}
	if t.Password != "" {
		q.Set("password", t.Password)
	}
	if t.Database != "" {
		q.Set("database", t.Database)
	}
	q.Set("conn_timeout", fmt.Sprintf("%d", int(dialTimeout.Seconds())))
	u.RawQuery = q.Encode()
	return u.String()
}
