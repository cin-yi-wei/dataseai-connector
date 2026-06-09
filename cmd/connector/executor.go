package main

import (
	"context"
	"fmt"
	"time"

	"github.com/cin-yi-wei/dataseai-connector/pkg/protocol"
)

// Sink is what an Executor writes its result stream to. The main loop is
// responsible for sending QueryDone / QueryError after Run returns; the
// executor only emits meta + row batches.
type Sink interface {
	Meta(cols []protocol.ColInfo) error
	Rows(batch [][]any) error
}

// Executor runs one query against a target and pushes results into sink.
type Executor interface {
	Run(ctx context.Context, req protocol.QueryRequest, sink Sink) error
}

// dialectRouter routes each request to the appropriate executor based on
// the Dialect field in the QueryRequest.
type dialectRouter struct {
	mysql     Executor
	postgres  Executor
	bytehouse Executor
	sqlite    Executor
}

func (r dialectRouter) Run(ctx context.Context, req protocol.QueryRequest, sink Sink) error {
	switch req.Dialect {
	case "postgres", "postgresql", "cockroachdb", "redshift":
		return r.postgres.Run(ctx, req, sink)
	case "bytehouse":
		return r.bytehouse.Run(ctx, req, sink)
	case "sqlite":
		return r.sqlite.Run(ctx, req, sink)
	default:
		return r.mysql.Run(ctx, req, sink)
	}
}

// MockExecutor pretends to be MySQL. Returns 50 fake rows in 5 batches of
// 10, sleeping 100ms between batches to simulate work. Used until the real
// MySQL executor lands.
type MockExecutor struct{}

func (MockExecutor) Run(ctx context.Context, req protocol.QueryRequest, sink Sink) error {
	if err := sink.Meta([]protocol.ColInfo{
		{Name: "id", Type: "INT"},
		{Name: "name", Type: "VARCHAR(64)"},
		{Name: "email", Type: "VARCHAR(128)"},
	}); err != nil {
		return err
	}
	const batches, perBatch = 5, 10
	for b := 0; b < batches; b++ {
		rows := make([][]any, 0, perBatch)
		for i := 0; i < perBatch; i++ {
			id := b*perBatch + i + 1
			rows = append(rows, []any{
				id,
				fmt.Sprintf("user_%d", id),
				fmt.Sprintf("u%d@example.com", id),
			})
		}
		if err := sink.Rows(rows); err != nil {
			return err
		}
		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
