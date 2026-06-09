package main

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/cin-yi-wei/dataseai-connector/pkg/protocol"
	_ "modernc.org/sqlite"
)

// SQLiteExecutor runs queries against a SQLite database file on the machine
// where the connector is running. The Host field of the target carries the
// file path (e.g. "/var/data/app.db" or ":memory:"). Port, user, and password
// are ignored — SQLite has no network layer.
type SQLiteExecutor struct{}

func (e SQLiteExecutor) Run(ctx context.Context, req protocol.QueryRequest, sink Sink) error {
	path := req.Target.Host
	if path == "" {
		return fmt.Errorf("sqlite: Host field must contain the database file path")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("sqlite open: %w", err)
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
