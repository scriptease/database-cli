package daemon

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/scriptease/jdbc-cli/internal/protocol"
)

type Store struct {
	mu      sync.RWMutex
	aliases map[string]*aliasState
}

type aliasState struct {
	mu       sync.Mutex
	db       *sql.DB
	tx       *sql.Tx
	readOnly bool
	dialect  dialect
	closed   bool
}

func newStore() *Store {
	return &Store{
		aliases: make(map[string]*aliasState),
	}
}

func (s *Store) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	aliases := make([]string, 0, len(s.aliases))
	for alias, state := range s.aliases {
		if state != nil && !state.closed {
			aliases = append(aliases, alias)
		}
	}
	sort.Strings(aliases)
	return aliases
}

func (s *Store) Open(req protocol.OpenRequest) error {
	alias := strings.TrimSpace(req.Alias)
	if alias == "" {
		return fmt.Errorf("alias is required")
	}
	if strings.TrimSpace(req.JDBCURL) == "" {
		return fmt.Errorf("jdbcUrl is required")
	}

	if req.PasswordKeychain != "" {
		password, err := lookupKeychainPassword(req.PasswordKeychain)
		if err != nil {
			return err
		}
		req.Password = password
	}

	config, err := parseJDBCURL(req.JDBCURL, req.User, req.Password)
	if err != nil {
		return err
	}

	db, err := sql.Open(config.DriverName, config.DSN)
	if err != nil {
		return err
	}
	if config.SingleConnection {
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		db.SetConnMaxLifetime(0)
	} else {
		db.SetMaxOpenConns(5)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(30 * time.Minute)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return err
	}

	state := &aliasState{
		db:       db,
		readOnly: req.ReadOnly,
		dialect:  config.Dialect,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.aliases[alias]; exists {
		_ = db.Close()
		return fmt.Errorf("alias already open: %s", alias)
	}
	s.aliases[alias] = state
	return nil
}

func (s *Store) Close(alias string) error {
	trimmedAlias := strings.TrimSpace(alias)
	state, err := s.get(trimmedAlias)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.tx != nil {
		return fmt.Errorf("active transaction on alias: %s", trimmedAlias)
	}

	delete(s.aliases, trimmedAlias)
	state.closed = true
	return state.db.Close()
}

func (s *Store) Query(alias string, sqlText string, jsonMode bool) ([]byte, string, error) {
	if strings.TrimSpace(sqlText) == "" {
		return nil, "", fmt.Errorf("sql is required")
	}

	trimmedAlias := strings.TrimSpace(alias)
	state, err := s.get(trimmedAlias)
	if err != nil {
		return nil, "", err
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if state.closed {
		return nil, "", fmt.Errorf("alias not open: %s", trimmedAlias)
	}

	rows, err := state.queryer().QueryContext(context.Background(), sqlText)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	if jsonMode {
		body, err := rowsToJSON(rows)
		return body, "application/json", err
	}

	body, err := rowsToTSV(rows)
	return body, "text/plain; charset=utf-8", err
}

func (s *Store) Exec(alias string, sqlText string) ([]byte, error) {
	if strings.TrimSpace(sqlText) == "" {
		return nil, fmt.Errorf("sql is required")
	}

	trimmedAlias := strings.TrimSpace(alias)
	state, err := s.get(trimmedAlias)
	if err != nil {
		return nil, err
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if state.closed {
		return nil, fmt.Errorf("alias not open: %s", trimmedAlias)
	}
	if state.readOnly {
		return nil, fmt.Errorf("alias is read-only: %s", trimmedAlias)
	}

	result, err := state.execer().ExecContext(context.Background(), sqlText)
	if err != nil {
		return nil, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		rowsAffected = 0
	}
	return marshalJSON(map[string]int64{"rowsAffected": rowsAffected})
}

func (s *Store) Schema(alias string) ([]byte, error) {
	trimmedAlias := strings.TrimSpace(alias)
	state, err := s.get(trimmedAlias)
	if err != nil {
		return nil, err
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if state.closed {
		return nil, fmt.Errorf("alias not open: %s", trimmedAlias)
	}

	query, args := schemaQuery(state.dialect)
	rows, err := state.queryer().QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tables := make([]string, 0)
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return marshalJSON(tables)
}

func (s *Store) Describe(alias string, table string) ([]byte, error) {
	if strings.TrimSpace(table) == "" {
		return nil, fmt.Errorf("table is required")
	}

	trimmedAlias := strings.TrimSpace(alias)
	state, err := s.get(trimmedAlias)
	if err != nil {
		return nil, err
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if state.closed {
		return nil, fmt.Errorf("alias not open: %s", trimmedAlias)
	}

	columns, err := describeColumns(state, table)
	if err != nil {
		return nil, err
	}
	return marshalJSON(columns)
}

func (s *Store) Begin(alias string) error {
	trimmedAlias := strings.TrimSpace(alias)
	state, err := s.get(trimmedAlias)
	if err != nil {
		return err
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if state.closed {
		return fmt.Errorf("alias not open: %s", trimmedAlias)
	}
	if state.tx != nil {
		return fmt.Errorf("transaction already open: %s", trimmedAlias)
	}

	tx, err := state.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	state.tx = tx
	return nil
}

func (s *Store) Commit(alias string) error {
	trimmedAlias := strings.TrimSpace(alias)
	state, err := s.get(trimmedAlias)
	if err != nil {
		return err
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if state.closed {
		return fmt.Errorf("alias not open: %s", trimmedAlias)
	}
	if state.tx == nil {
		return fmt.Errorf("no active transaction: %s", trimmedAlias)
	}

	tx := state.tx
	state.tx = nil
	return tx.Commit()
}

func (s *Store) Rollback(alias string) error {
	trimmedAlias := strings.TrimSpace(alias)
	state, err := s.get(trimmedAlias)
	if err != nil {
		return err
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if state.closed {
		return fmt.Errorf("alias not open: %s", trimmedAlias)
	}
	if state.tx == nil {
		return fmt.Errorf("no active transaction: %s", trimmedAlias)
	}

	tx := state.tx
	state.tx = nil
	return tx.Rollback()
}

func (s *Store) Batch(body []byte) ([]byte, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 1024), 10*1024*1024)

	var out bytes.Buffer
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		out.Write(s.batchLine(line))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (s *Store) CloseAll() {
	s.mu.Lock()
	aliases := s.aliases
	s.aliases = make(map[string]*aliasState)
	s.mu.Unlock()

	for _, state := range aliases {
		if state == nil {
			continue
		}

		state.mu.Lock()
		if state.tx != nil {
			_ = state.tx.Rollback()
			state.tx = nil
		}
		state.closed = true
		db := state.db
		state.mu.Unlock()

		if db != nil {
			_ = db.Close()
		}
	}
}

func (s *Store) get(alias string) (*aliasState, error) {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return nil, fmt.Errorf("alias is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	state := s.aliases[alias]
	if state == nil {
		return nil, fmt.Errorf("alias not open: %s", alias)
	}
	return state, nil
}

func (s *Store) batchLine(line string) []byte {
	var op protocol.BatchOp
	if err := json.Unmarshal([]byte(line), &op); err != nil {
		return errorJSON(err)
	}

	switch op.Op {
	case "query":
		body, _, err := s.Query(op.Alias, op.SQL, op.JSON)
		if err != nil {
			return errorJSON(err)
		}
		return body
	case "exec":
		body, err := s.Exec(op.Alias, op.SQL)
		if err != nil {
			return errorJSON(err)
		}
		return body
	case "begin":
		if err := s.Begin(op.Alias); err != nil {
			return errorJSON(err)
		}
		body, err := okJSON()
		if err != nil {
			return errorJSON(err)
		}
		return body
	case "commit":
		if err := s.Commit(op.Alias); err != nil {
			return errorJSON(err)
		}
		body, err := okJSON()
		if err != nil {
			return errorJSON(err)
		}
		return body
	case "rollback":
		if err := s.Rollback(op.Alias); err != nil {
			return errorJSON(err)
		}
		body, err := okJSON()
		if err != nil {
			return errorJSON(err)
		}
		return body
	default:
		return errorJSON(fmt.Errorf("unknown op: %s", op.Op))
	}
}

func (a *aliasState) queryer() interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
} {
	if a.tx != nil {
		return a.tx
	}
	return a.db
}

func (a *aliasState) execer() interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
} {
	if a.tx != nil {
		return a.tx
	}
	return a.db
}

func schemaQuery(d dialect) (string, []any) {
	switch d {
	case dialectMySQL:
		return "SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE' ORDER BY table_name", nil
	case dialectPostgres:
		return "SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema() AND table_type = 'BASE TABLE' ORDER BY table_name", nil
	case dialectSQLite:
		return "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name", nil
	default:
		return "", nil
	}
}

func describeColumns(state *aliasState, table string) ([]describeColumn, error) {
	switch state.dialect {
	case dialectMySQL:
		return describeInformationSchema(state, table, "SELECT column_name, column_type, COALESCE(character_maximum_length, numeric_precision, datetime_precision, 0) AS size, CASE WHEN is_nullable = 'YES' THEN 1 ELSE 0 END AS nullable FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? ORDER BY ordinal_position")
	case dialectPostgres:
		return describeInformationSchema(state, table, "SELECT column_name, udt_name, COALESCE(character_maximum_length, numeric_precision, datetime_precision, 0) AS size, CASE WHEN is_nullable = 'YES' THEN 1 ELSE 0 END AS nullable FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1 ORDER BY ordinal_position")
	case dialectSQLite:
		return describeSQLite(state, table)
	default:
		return nil, fmt.Errorf("unsupported dialect: %s", state.dialect)
	}
}

func describeInformationSchema(state *aliasState, table string, query string) ([]describeColumn, error) {
	rows, err := state.queryer().QueryContext(context.Background(), query, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make([]describeColumn, 0)
	for rows.Next() {
		var name string
		var typeName string
		var size sql.NullInt64
		var nullable int64
		if err := rows.Scan(&name, &typeName, &size, &nullable); err != nil {
			return nil, err
		}
		columns = append(columns, describeColumn{
			Name:     name,
			Type:     typeName,
			Size:     size.Int64,
			Nullable: nullable == 1,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

func describeSQLite(state *aliasState, table string) ([]describeColumn, error) {
	quotedTable := strings.ReplaceAll(table, `"`, `""`)
	query := fmt.Sprintf(`PRAGMA table_info("%s")`, quotedTable)

	rows, err := state.queryer().QueryContext(context.Background(), query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make([]describeColumn, 0)
	for rows.Next() {
		var cid int64
		var name string
		var typeName string
		var notNull int64
		var defaultValue sql.NullString
		var pk int64
		if err := rows.Scan(&cid, &name, &typeName, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		columns = append(columns, describeColumn{
			Name:     name,
			Type:     typeName,
			Size:     sqliteColumnSize(typeName),
			Nullable: notNull == 0,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}
