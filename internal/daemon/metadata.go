package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

func (a *aliasState) schema(alias string) ([]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.requireOpen(alias); err != nil {
		return nil, err
	}

	query, args := schemaQuery(a.dialect)
	rows, err := a.queryer().QueryContext(context.Background(), query, args...)
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

func (a *aliasState) describe(alias string, table string) ([]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.requireOpen(alias); err != nil {
		return nil, err
	}

	columns, err := describeColumns(a, table)
	if err != nil {
		return nil, err
	}
	return marshalJSON(columns)
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
