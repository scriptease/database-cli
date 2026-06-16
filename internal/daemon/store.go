package daemon

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/scriptease/database-cli/internal/protocol"
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
	trimmedAlias, state, err := s.getState(alias)
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

	trimmedAlias, state, err := s.getState(alias)
	if err != nil {
		return nil, "", err
	}
	return state.query(trimmedAlias, sqlText, jsonMode)
}

func (s *Store) Exec(alias string, sqlText string) ([]byte, error) {
	if strings.TrimSpace(sqlText) == "" {
		return nil, fmt.Errorf("sql is required")
	}

	trimmedAlias, state, err := s.getState(alias)
	if err != nil {
		return nil, err
	}
	return state.exec(trimmedAlias, sqlText)
}

func (s *Store) Schema(alias string) ([]byte, error) {
	trimmedAlias, state, err := s.getState(alias)
	if err != nil {
		return nil, err
	}
	return state.schema(trimmedAlias)
}

func (s *Store) Describe(alias string, table string) ([]byte, error) {
	if strings.TrimSpace(table) == "" {
		return nil, fmt.Errorf("table is required")
	}

	trimmedAlias, state, err := s.getState(alias)
	if err != nil {
		return nil, err
	}
	return state.describe(trimmedAlias, table)
}

func (s *Store) Begin(alias string) error {
	trimmedAlias, state, err := s.getState(alias)
	if err != nil {
		return err
	}
	return state.begin(trimmedAlias)
}

func (s *Store) Commit(alias string) error {
	trimmedAlias, state, err := s.getState(alias)
	if err != nil {
		return err
	}
	return state.commit(trimmedAlias)
}

func (s *Store) Rollback(alias string) error {
	trimmedAlias, state, err := s.getState(alias)
	if err != nil {
		return err
	}
	return state.rollback(trimmedAlias)
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

func (s *Store) getState(alias string) (string, *aliasState, error) {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return "", nil, fmt.Errorf("alias is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	state := s.aliases[alias]
	if state == nil {
		return "", nil, fmt.Errorf("alias not open: %s", alias)
	}
	return alias, state, nil
}
