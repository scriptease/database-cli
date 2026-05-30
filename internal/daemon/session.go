package daemon

import (
	"context"
	"database/sql"
	"fmt"
)

func (a *aliasState) query(alias string, sqlText string, jsonMode bool) ([]byte, string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.requireOpen(alias); err != nil {
		return nil, "", err
	}

	rows, err := a.queryer().QueryContext(context.Background(), sqlText)
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

func (a *aliasState) exec(alias string, sqlText string) ([]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.requireOpen(alias); err != nil {
		return nil, err
	}
	if err := a.requireWritable(alias); err != nil {
		return nil, err
	}

	result, err := a.execer().ExecContext(context.Background(), sqlText)
	if err != nil {
		return nil, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		rowsAffected = 0
	}
	return marshalJSON(map[string]int64{"rowsAffected": rowsAffected})
}

func (a *aliasState) begin(alias string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.requireOpen(alias); err != nil {
		return err
	}
	if err := a.requireWritable(alias); err != nil {
		return err
	}
	if a.tx != nil {
		return fmt.Errorf("transaction already open: %s", alias)
	}

	tx, err := a.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	a.tx = tx
	return nil
}

func (a *aliasState) commit(alias string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.requireOpen(alias); err != nil {
		return err
	}
	if err := a.requireWritable(alias); err != nil {
		return err
	}
	if a.tx == nil {
		return fmt.Errorf("no active transaction: %s", alias)
	}

	tx := a.tx
	a.tx = nil
	return tx.Commit()
}

func (a *aliasState) rollback(alias string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.requireOpen(alias); err != nil {
		return err
	}
	if err := a.requireWritable(alias); err != nil {
		return err
	}
	if a.tx == nil {
		return fmt.Errorf("no active transaction: %s", alias)
	}

	tx := a.tx
	a.tx = nil
	return tx.Rollback()
}

func (a *aliasState) requireOpen(alias string) error {
	if a.closed {
		return fmt.Errorf("alias not open: %s", alias)
	}
	return nil
}

func (a *aliasState) requireWritable(alias string) error {
	if a.readOnly {
		return fmt.Errorf("alias is read-only: %s", alias)
	}
	return nil
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
