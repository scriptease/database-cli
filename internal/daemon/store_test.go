package daemon

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scriptease/jdbc-cli/internal/protocol"
)

func TestStoreSQLiteLifecycle(t *testing.T) {
	store := newStore()
	t.Cleanup(store.CloseAll)

	dbPath := filepath.Join(t.TempDir(), "app.db")
	if err := store.Open(protocol.OpenRequest{
		Alias:   "app",
		JDBCURL: "jdbc:sqlite:" + dbPath,
	}); err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if _, err := store.Exec("app", "create table users(id integer primary key, name text not null)"); err != nil {
		t.Fatalf("Exec(create table) error = %v", err)
	}
	if _, err := store.Exec("app", "insert into users(name) values ('Ada')"); err != nil {
		t.Fatalf("Exec(insert) error = %v", err)
	}

	tsvBody, _, err := store.Query("app", "select id, name from users order by id", false)
	if err != nil {
		t.Fatalf("Query(tsv) error = %v", err)
	}
	if string(tsvBody) != "id\tname\n1\tAda\n" {
		t.Fatalf("Query(tsv) = %q, want %q", string(tsvBody), "id\tname\n1\tAda\n")
	}

	jsonBody, _, err := store.Query("app", "select id, name from users order by id", true)
	if err != nil {
		t.Fatalf("Query(json) error = %v", err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(jsonBody, &rows); err != nil {
		t.Fatalf("json.Unmarshal(query) error = %v", err)
	}
	if len(rows) != 1 || rows[0]["name"] != "Ada" || rows[0]["id"].(float64) != 1 {
		t.Fatalf("query rows = %#v, want Ada row", rows)
	}

	schemaBody, err := store.Schema("app")
	if err != nil {
		t.Fatalf("Schema() error = %v", err)
	}
	var tables []string
	if err := json.Unmarshal(schemaBody, &tables); err != nil {
		t.Fatalf("json.Unmarshal(schema) error = %v", err)
	}
	if len(tables) != 1 || tables[0] != "users" {
		t.Fatalf("schema tables = %#v, want [users]", tables)
	}

	describeBody, err := store.Describe("app", "users")
	if err != nil {
		t.Fatalf("Describe() error = %v", err)
	}
	var columns []describeColumn
	if err := json.Unmarshal(describeBody, &columns); err != nil {
		t.Fatalf("json.Unmarshal(describe) error = %v", err)
	}
	if len(columns) != 2 || columns[0].Name != "id" || columns[1].Name != "name" {
		t.Fatalf("describe columns = %#v, want id/name", columns)
	}

	if err := store.Begin("app"); err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if _, err := store.Exec("app", "insert into users(name) values ('Grace')"); err != nil {
		t.Fatalf("Exec(tx insert) error = %v", err)
	}
	if err := store.Rollback("app"); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}

	countBody, _, err := store.Query("app", "select count(*) as count from users", true)
	if err != nil {
		t.Fatalf("Query(count) error = %v", err)
	}
	var countRows []map[string]any
	if err := json.Unmarshal(countBody, &countRows); err != nil {
		t.Fatalf("json.Unmarshal(count) error = %v", err)
	}
	if len(countRows) != 1 || countRows[0]["count"].(float64) != 1 {
		t.Fatalf("count rows = %#v, want count=1", countRows)
	}
}

func TestStoreSQLiteMemoryPersistsAcrossCommands(t *testing.T) {
	store := newStore()
	t.Cleanup(store.CloseAll)

	if err := store.Open(protocol.OpenRequest{
		Alias:   "mem",
		JDBCURL: "jdbc:sqlite::memory:",
	}); err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if _, err := store.Exec("mem", "create table t(id integer primary key, name text)"); err != nil {
		t.Fatalf("Exec(create table) error = %v", err)
	}
	if _, err := store.Exec("mem", "insert into t(name) values ('Ada')"); err != nil {
		t.Fatalf("Exec(insert) error = %v", err)
	}

	body, _, err := store.Query("mem", "select count(*) as count from t", true)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(rows) != 1 || rows[0]["count"].(float64) != 1 {
		t.Fatalf("rows = %#v, want count=1", rows)
	}
}

func TestStoreReadOnlyRejectsExecAndCloseRejectsActiveTransaction(t *testing.T) {
	store := newStore()
	t.Cleanup(store.CloseAll)

	dbPath := filepath.Join(t.TempDir(), "app.db")
	if err := store.Open(protocol.OpenRequest{
		Alias:   "rw",
		JDBCURL: "jdbc:sqlite:" + dbPath,
	}); err != nil {
		t.Fatalf("Open(rw) error = %v", err)
	}
	if _, err := store.Exec("rw", "create table items(id integer primary key, name text)"); err != nil {
		t.Fatalf("Exec(create table) error = %v", err)
	}

	if err := store.Begin("rw"); err != nil {
		t.Fatalf("Begin(rw) error = %v", err)
	}
	if err := store.Close("rw"); err == nil || !strings.Contains(err.Error(), "active transaction") {
		t.Fatalf("Close(rw) error = %v, want active transaction error", err)
	}
	if err := store.Rollback("rw"); err != nil {
		t.Fatalf("Rollback(rw) error = %v", err)
	}

	if err := store.Open(protocol.OpenRequest{
		Alias:    "ro",
		JDBCURL:  "jdbc:sqlite:" + dbPath,
		ReadOnly: true,
	}); err != nil {
		t.Fatalf("Open(ro) error = %v", err)
	}

	if _, err := store.Exec("ro", "insert into items(name) values ('blocked')"); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("Exec(ro) error = %v, want read-only error", err)
	}
}

func TestBatchQueryHonorsJSONFlag(t *testing.T) {
	store := newStore()
	t.Cleanup(store.CloseAll)

	if err := store.Open(protocol.OpenRequest{
		Alias:   "mem",
		JDBCURL: "jdbc:sqlite::memory:",
	}); err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if _, err := store.Exec("mem", "create table t(id integer primary key, name text)"); err != nil {
		t.Fatalf("Exec(create table) error = %v", err)
	}
	if _, err := store.Exec("mem", "insert into t(name) values ('Ada')"); err != nil {
		t.Fatalf("Exec(insert) error = %v", err)
	}

	body, err := store.Batch([]byte(
		"{\"op\":\"query\",\"alias\":\"mem\",\"sql\":\"select id, name from t order by id\"}\n" +
			"{\"op\":\"query\",\"alias\":\"mem\",\"sql\":\"select id, name from t order by id\",\"json\":true}\n",
	))
	if err != nil {
		t.Fatalf("Batch() error = %v", err)
	}

	want := "id\tname\n1\tAda\n[{\"id\":1,\"name\":\"Ada\"}]\n"
	if string(body) != want {
		t.Fatalf("Batch() = %q, want %q", string(body), want)
	}
}
