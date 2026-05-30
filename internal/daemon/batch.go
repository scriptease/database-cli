package daemon

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/scriptease/jdbc-cli/internal/protocol"
)

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
		return okLine()
	case "commit":
		if err := s.Commit(op.Alias); err != nil {
			return errorJSON(err)
		}
		return okLine()
	case "rollback":
		if err := s.Rollback(op.Alias); err != nil {
			return errorJSON(err)
		}
		return okLine()
	default:
		return errorJSON(fmt.Errorf("unknown op: %s", op.Op))
	}
}

func okLine() []byte {
	body, err := okJSON()
	if err != nil {
		return errorJSON(err)
	}
	return body
}
