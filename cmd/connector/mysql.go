package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/cin-yi-wei/dataseai-connector/pkg/protocol"
	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/ssh"
)

// MySQLExecutor runs the query against the target MySQL server using
// credentials supplied in the request.
//
// V0 caveats:
//   - Opens a fresh connection per request (no pooling).
//   - Credentials arrive in plaintext over the (TLS) WebSocket from the
//     broker. Storing creds on the connector locally is a later refinement.
//   - Read-only path; writes will require a separate proposal/audit flow.
type MySQLExecutor struct {
	// DialTimeout caps how long we wait to open the underlying TCP
	// connection. The query itself is bounded by the request context.
	DialTimeout time.Duration
}

func (e MySQLExecutor) Run(ctx context.Context, req protocol.QueryRequest, sink Sink) error {
	var sshClient *ssh.Client
	dialerName := ""
	if !req.Target.SSH.IsZero() {
		var err error
		sshClient, err = openSSHClient(*req.Target.SSH, e.DialTimeout)
		if err != nil {
			return err
		}
		defer sshClient.Close()
		dialerName = nextSSHDialerName()
		mysql.RegisterDialContext(dialerName, sshDialer(sshClient))
	}
	dsn := buildDSN(req.Target, e.DialTimeout, dialerName)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("open: %w", err)
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
		meta[i] = protocol.ColInfo{
			Name: c.Name(),
			Type: c.DatabaseTypeName(),
		}
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

func buildDSN(t protocol.MySQLTarget, dialTimeout time.Duration, dialerName string) string {
	return buildMySQLConfig(t, dialTimeout, dialerName).FormatDSN()
}

func buildMySQLConfig(t protocol.MySQLTarget, dialTimeout time.Duration, dialerName string) *mysql.Config {
	if dialTimeout <= 0 {
		dialTimeout = 10 * time.Second
	}
	addr := net.JoinHostPort(t.Host, strconv.Itoa(t.Port))
	cfg := mysql.NewConfig()
	cfg.User = t.User
	cfg.Passwd = t.Password
	cfg.Net = "tcp"
	if dialerName != "" {
		cfg.Net = dialerName
	}
	cfg.Addr = addr
	cfg.DBName = t.Database
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.Timeout = dialTimeout
	cfg.Params = map[string]string{
		"charset": "utf8mb4",
	}
	return cfg
}

var sshDialerCounter uint64

func nextSSHDialerName() string {
	n := atomic.AddUint64(&sshDialerCounter, 1)
	return fmt.Sprintf("ssh-%d-%d", time.Now().UnixNano(), n)
}

func openSSHClient(cfg protocol.SSHConfig, dialTimeout time.Duration) (*ssh.Client, error) {
	if cfg.IsZero() {
		return nil, fmt.Errorf("ssh: host/user required")
	}
	if dialTimeout <= 0 {
		dialTimeout = 10 * time.Second
	}
	port := cfg.Port
	if port == 0 {
		port = 22
	}
	auth, err := sshAuthMethod(cfg)
	if err != nil {
		return nil, err
	}
	sshCfg := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         dialTimeout,
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(port))
	client, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}
	return client, nil
}

func sshAuthMethod(cfg protocol.SSHConfig) (ssh.AuthMethod, error) {
	if cfg.PrivateKey != "" {
		var signer ssh.Signer
		var err error
		if cfg.KeyPassphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(cfg.PrivateKey), []byte(cfg.KeyPassphrase))
		} else {
			signer, err = ssh.ParsePrivateKey([]byte(cfg.PrivateKey))
		}
		if err != nil {
			return nil, fmt.Errorf("ssh key parse: %w", err)
		}
		return ssh.PublicKeys(signer), nil
	}
	if cfg.Password == "" {
		return nil, fmt.Errorf("ssh: password or private key required")
	}
	return ssh.Password(cfg.Password), nil
}

func sshDialer(client *ssh.Client) func(context.Context, string) (net.Conn, error) {
	return func(ctx context.Context, addr string) (net.Conn, error) {
		type result struct {
			conn net.Conn
			err  error
		}
		ch := make(chan result, 1)
		go func() {
			conn, err := client.Dial("tcp", addr)
			ch <- result{conn: conn, err: err}
		}()
		select {
		case res := <-ch:
			return res.conn, res.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// jsonable converts driver scan values (which may be []byte for strings,
// time.Time for dates, etc.) into something json.Marshal will produce
// reasonable output for.
func jsonable(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case []byte:
		return string(x)
	case time.Time:
		return x.Format(time.RFC3339)
	default:
		return v
	}
}
