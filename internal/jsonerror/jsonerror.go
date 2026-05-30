package jsonerror

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

type Raw struct {
	Body []byte
}

func (r Raw) Error() string {
	return string(bytes.TrimSpace(r.Body))
}

func NewRaw(body []byte) error {
	return Raw{Body: append([]byte(nil), body...)}
}

func Write(w io.Writer, err error) {
	var raw Raw
	if errors.As(err, &raw) {
		body := bytes.TrimSpace(raw.Body)
		if len(body) == 0 {
			body = []byte(`{"error":"request failed"}`)
		}
		_, _ = w.Write(body)
		if body[len(body)-1] != '\n' {
			_, _ = w.Write([]byte("\n"))
		}
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
}
