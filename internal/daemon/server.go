package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/scriptease/jdbc-cli/internal/protocol"
)

func Run(args []string) error {
	if len(args) > 0 {
		if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
			_, _ = fmt.Fprintln(os.Stdout, "jdbc-cli daemon")
			return nil
		}
		return fmt.Errorf("daemon does not accept arguments")
	}

	stateDir, err := protocol.StateDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}

	socketPath, err := protocol.SocketPath()
	if err != nil {
		return err
	}
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(socketPath)
	}()

	if err := os.Chmod(socketPath, 0o600); err != nil {
		return err
	}

	store := newStore()
	server := &http.Server{Handler: routes(store)}
	serverErrors := make(chan error, 1)

	go func() {
		serverErrors <- server.Serve(listener)
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	select {
	case sig := <-signals:
		_ = sig
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		store.CloseAll()
		err := <-serverErrors
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case err := <-serverErrors:
		store.CloseAll()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func routes(store *Store) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc(protocol.PathPing, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		writeText(w, http.StatusOK, []byte("ok\n"))
	})

	mux.HandleFunc(protocol.PathList, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		writeJSON(w, http.StatusOK, store.List())
	})

	mux.HandleFunc(protocol.PathOpen, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		var req protocol.OpenRequest
		if err := decodeJSON(r.Body, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := store.Open(req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeOK(w)
	})

	mux.HandleFunc(protocol.PathClose, func(w http.ResponseWriter, r *http.Request) {
		handleAliasMutation(w, r, store.Close)
	})
	mux.HandleFunc(protocol.PathSchema, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		var req protocol.AliasRequest
		if err := decodeJSON(r.Body, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		body, err := store.Schema(req.Alias)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSONBytes(w, http.StatusOK, body)
	})
	mux.HandleFunc(protocol.PathDescribe, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		var req protocol.DescribeRequest
		if err := decodeJSON(r.Body, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		body, err := store.Describe(req.Alias, req.Table)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSONBytes(w, http.StatusOK, body)
	})
	mux.HandleFunc(protocol.PathQuery, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		var req protocol.SQLRequest
		if err := decodeJSON(r.Body, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		body, contentType, err := store.Query(req.Alias, req.SQL, req.JSON || r.URL.Query().Get("json") == "1")
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeBody(w, http.StatusOK, contentType, body)
	})
	mux.HandleFunc(protocol.PathExec, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		var req protocol.SQLRequest
		if err := decodeJSON(r.Body, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		body, err := store.Exec(req.Alias, req.SQL)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSONBytes(w, http.StatusOK, body)
	})
	mux.HandleFunc(protocol.PathBegin, func(w http.ResponseWriter, r *http.Request) {
		handleAliasMutation(w, r, store.Begin)
	})
	mux.HandleFunc(protocol.PathCommit, func(w http.ResponseWriter, r *http.Request) {
		handleAliasMutation(w, r, store.Commit)
	})
	mux.HandleFunc(protocol.PathRollback, func(w http.ResponseWriter, r *http.Request) {
		handleAliasMutation(w, r, store.Rollback)
	})
	mux.HandleFunc(protocol.PathBatch, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		response, err := store.Batch(body)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeBody(w, http.StatusOK, "application/x-ndjson", response)
	})

	return mux
}

func handleAliasMutation(w http.ResponseWriter, r *http.Request, fn func(string) error) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}
	var req protocol.AliasRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := fn(req.Alias); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeOK(w)
}

func decodeJSON(body io.Reader, target any) error {
	decoder := json.NewDecoder(body)
	return decoder.Decode(target)
}

func writeOK(w http.ResponseWriter) {
	body, err := okJSON()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONBytes(w, http.StatusOK, body)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	body, err := marshalJSON(payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONBytes(w, status, body)
}

func writeJSONBytes(w http.ResponseWriter, status int, body []byte) {
	writeBody(w, status, "application/json", body)
}

func writeText(w http.ResponseWriter, status int, body []byte) {
	writeBody(w, status, "text/plain; charset=utf-8", body)
}

func writeBody(w http.ResponseWriter, status int, contentType string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
