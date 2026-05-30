package protocol

import (
	"os"
	"path/filepath"
)

const (
	PathPing     = "/ping"
	PathList     = "/list"
	PathOpen     = "/open"
	PathClose    = "/close"
	PathQuery    = "/query"
	PathExec     = "/exec"
	PathSchema   = "/schema"
	PathDescribe = "/describe"
	PathBegin    = "/begin"
	PathCommit   = "/commit"
	PathRollback = "/rollback"
	PathBatch    = "/batch"
)

type OpenRequest struct {
	Alias            string `json:"alias"`
	JDBCURL          string `json:"jdbcUrl"`
	User             string `json:"user,omitempty"`
	Password         string `json:"password,omitempty"`
	PasswordKeychain string `json:"passwordKeychain,omitempty"`
	ReadOnly         bool   `json:"readOnly,omitempty"`
}

type AliasRequest struct {
	Alias string `json:"alias"`
}

type SQLRequest struct {
	Alias string `json:"alias"`
	SQL   string `json:"sql"`
	JSON  bool   `json:"json,omitempty"`
}

type DescribeRequest struct {
	Alias string `json:"alias"`
	Table string `json:"table"`
}

type BatchOp struct {
	Op    string `json:"op"`
	Alias string `json:"alias,omitempty"`
	SQL   string `json:"sql,omitempty"`
	JSON  bool   `json:"json,omitempty"`
	Table string `json:"table,omitempty"`
}

func StateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".jdbc-cli"), nil
}

func SocketPath() (string, error) {
	stateDir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(stateDir, "sock"), nil
}
