package daemon

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var sqliteSizePattern = regexp.MustCompile(`\((\d+)`)

type describeColumn struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Size     int64  `json:"size"`
	Nullable bool   `json:"nullable"`
}

func marshalJSON(v any) ([]byte, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

func okJSON() ([]byte, error) {
	return marshalJSON(map[string]bool{"ok": true})
}

func errorJSON(err error) []byte {
	body, marshalErr := marshalJSON(map[string]string{"error": err.Error()})
	if marshalErr != nil {
		return []byte("{\"error\":\"internal error\"}\n")
	}
	return body
}

func rowsToTSV(rows *sql.Rows) ([]byte, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var builder strings.Builder
	builder.WriteString(strings.Join(columns, "\t"))
	builder.WriteByte('\n')

	values := make([]any, len(columns))
	pointers := make([]any, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}

	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		for i, value := range values {
			if i > 0 {
				builder.WriteByte('\t')
			}
			builder.WriteString(tsvCell(value))
		}
		builder.WriteByte('\n')
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return []byte(builder.String()), nil
}

func rowsToJSON(rows *sql.Rows) ([]byte, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}

	values := make([]any, len(columns))
	pointers := make([]any, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}

	result := make([]map[string]any, 0)
	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}

		item := make(map[string]any, len(columns))
		for i, column := range columns {
			item[column] = jsonCell(values[i], columnTypes[i])
		}
		result = append(result, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return marshalJSON(result)
}

func tsvCell(value any) string {
	switch v := value.(type) {
	case nil:
		return `\N`
	case []byte:
		return string(v)
	case time.Time:
		return v.Format(time.RFC3339Nano)
	default:
		return fmt.Sprint(v)
	}
}

func jsonCell(value any, columnType *sql.ColumnType) any {
	typeName := ""
	if columnType != nil {
		typeName = strings.ToUpper(columnType.DatabaseTypeName())
	}

	switch v := value.(type) {
	case nil:
		return nil
	case bool:
		return v
	case int:
		return v
	case int8:
		return v
	case int16:
		return v
	case int32:
		return v
	case int64:
		return v
	case uint:
		return v
	case uint8:
		return v
	case uint16:
		return v
	case uint32:
		return v
	case uint64:
		return v
	case float32:
		return v
	case float64:
		return v
	case time.Time:
		return v.Format(time.RFC3339Nano)
	case []byte:
		return jsonStringLikeValue(string(v), typeName, v)
	case string:
		return jsonStringLikeValue(v, typeName, []byte(v))
	default:
		return fmt.Sprint(v)
	}
}

func jsonStringLikeValue(value string, typeName string, raw []byte) any {
	switch {
	case isBinaryType(typeName):
		return base64.StdEncoding.EncodeToString(raw)
	case isDecimalType(typeName):
		return value
	case isIntegerType(typeName):
		if parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
			return parsed
		}
	case isFloatType(typeName):
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
			return parsed
		}
	case isBoolType(typeName):
		if parsed, ok := parseBoolish(value); ok {
			return parsed
		}
	}
	return value
}

func isBinaryType(typeName string) bool {
	switch typeName {
	case "BLOB", "BINARY", "VARBINARY", "BYTEA", "LONGBLOB", "MEDIUMBLOB", "TINYBLOB":
		return true
	default:
		return false
	}
}

func isDecimalType(typeName string) bool {
	switch typeName {
	case "DECIMAL", "NUMERIC":
		return true
	default:
		return false
	}
}

func isIntegerType(typeName string) bool {
	switch typeName {
	case "INT", "INTEGER", "INT2", "INT4", "INT8", "BIGINT", "SMALLINT", "TINYINT", "MEDIUMINT", "SERIAL", "BIGSERIAL":
		return true
	default:
		return false
	}
}

func isFloatType(typeName string) bool {
	switch typeName {
	case "FLOAT", "FLOAT4", "FLOAT8", "REAL", "DOUBLE", "DOUBLE PRECISION":
		return true
	default:
		return false
	}
}

func isBoolType(typeName string) bool {
	switch typeName {
	case "BOOL", "BOOLEAN":
		return true
	default:
		return false
	}
}

func parseBoolish(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "t", "true", "y", "yes":
		return true, true
	case "0", "f", "false", "n", "no":
		return false, true
	default:
		return false, false
	}
}

func sqliteColumnSize(typeName string) int64 {
	matches := sqliteSizePattern.FindStringSubmatch(typeName)
	if len(matches) != 2 {
		return 0
	}
	size, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return 0
	}
	return size
}
