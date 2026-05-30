package daemon

import (
	"net/url"
	"strings"
	"testing"
)

func TestParseMySQLJDBC(t *testing.T) {
	cfg, err := parseJDBCURL("jdbc:mysql://db.example.com:3307/app?tls=skip-verify", "alice", "secret")
	if err != nil {
		t.Fatalf("parseJDBCURL() error = %v", err)
	}

	if cfg.DriverName != "mysql" {
		t.Fatalf("driver = %q, want mysql", cfg.DriverName)
	}
	if cfg.Dialect != dialectMySQL {
		t.Fatalf("dialect = %q, want %q", cfg.Dialect, dialectMySQL)
	}
	if !strings.Contains(cfg.DSN, "alice:secret@tcp(db.example.com:3307)/app") {
		t.Fatalf("dsn = %q, want mysql tcp dsn with credentials", cfg.DSN)
	}
	if !strings.Contains(cfg.DSN, "parseTime=true") {
		t.Fatalf("dsn = %q, want parseTime=true", cfg.DSN)
	}
}

func TestParsePostgresJDBC(t *testing.T) {
	cfg, err := parseJDBCURL("jdbc:postgresql://db.example.com:5432/app?sslmode=disable", "alice", "secret")
	if err != nil {
		t.Fatalf("parseJDBCURL() error = %v", err)
	}

	if cfg.DriverName != "pgx" {
		t.Fatalf("driver = %q, want pgx", cfg.DriverName)
	}
	if cfg.Dialect != dialectPostgres {
		t.Fatalf("dialect = %q, want %q", cfg.Dialect, dialectPostgres)
	}

	parsed, err := url.Parse(cfg.DSN)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", cfg.DSN, err)
	}
	if parsed.User == nil || parsed.User.Username() != "alice" {
		t.Fatalf("dsn user = %v, want alice", parsed.User)
	}
	password, _ := parsed.User.Password()
	if password != "secret" {
		t.Fatalf("dsn password = %q, want secret", password)
	}
}

func TestParseSQLiteMemoryUsesSingleConnection(t *testing.T) {
	cfg, err := parseJDBCURL("jdbc:sqlite::memory:", "", "")
	if err != nil {
		t.Fatalf("parseJDBCURL() error = %v", err)
	}

	if cfg.DriverName != "sqlite" {
		t.Fatalf("driver = %q, want sqlite", cfg.DriverName)
	}
	if cfg.Dialect != dialectSQLite {
		t.Fatalf("dialect = %q, want %q", cfg.Dialect, dialectSQLite)
	}
	if !cfg.SingleConnection {
		t.Fatal("expected sqlite :memory: to force a single pooled connection")
	}
}
