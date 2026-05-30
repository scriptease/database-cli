package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/scriptease/jdbc-cli/internal/client"
	"github.com/scriptease/jdbc-cli/internal/protocol"
)

const topHelp = `jdbc-cli <subcommand> [flags] [args]

Subcommands:
  ping                   Check if the daemon is running
  list                   List open connection aliases
  open                   Open a new JDBC connection
  close                  Close an open connection
  query                  Run a SELECT query
  exec                   Run an INSERT/UPDATE/DELETE statement
  schema                 List tables in the database
  describe               Describe columns of a table
  begin                  Begin a transaction
  commit                 Commit a transaction
  rollback               Roll back a transaction
  batch                  Execute multiple operations from stdin (NDJSON)

Global flags:
  --help                 Show this help

Run 'jdbc-cli <subcommand> --help' for subcommand-specific usage.
`

var boolFlags = map[string]struct{}{
	"help":           {},
	"json":           {},
	"password-stdin": {},
	"read-only":      {},
}

type parsedArgs struct {
	flags       map[string]string
	positionals []string
}

func Run(args []string) error {
	if len(args) == 0 || isHelp(args[0]) {
		printTopHelp()
		return nil
	}

	socketPath, err := protocol.SocketPath()
	if err != nil {
		return err
	}
	httpClient := client.New(socketPath)

	switch args[0] {
	case "ping":
		return runPing(httpClient, args[1:])
	case "list":
		return runList(httpClient, args[1:])
	case "open":
		return runOpen(httpClient, args[1:])
	case "close":
		return runAliasCommand(httpClient, args[1:], protocol.PathClose, "close")
	case "query":
		return runQuery(httpClient, args[1:])
	case "exec":
		return runExec(httpClient, args[1:])
	case "schema":
		return runAliasCommand(httpClient, args[1:], protocol.PathSchema, "schema")
	case "describe":
		return runDescribe(httpClient, args[1:])
	case "begin":
		return runAliasCommand(httpClient, args[1:], protocol.PathBegin, "begin")
	case "commit":
		return runAliasCommand(httpClient, args[1:], protocol.PathCommit, "commit")
	case "rollback":
		return runAliasCommand(httpClient, args[1:], protocol.PathRollback, "rollback")
	case "batch":
		return runBatch(httpClient, args[1:])
	default:
		return fmt.Errorf("unknown subcommand: %s", args[0])
	}
}

func runPing(httpClient *client.Client, args []string) error {
	if containsHelp(args) {
		printHelp("ping")
		return nil
	}

	body, err := httpClient.Get(protocol.PathPing)
	if err != nil {
		return err
	}
	writeStdout(body)
	return nil
}

func runList(httpClient *client.Client, args []string) error {
	if containsHelp(args) {
		printHelp("list")
		return nil
	}

	body, err := httpClient.Get(protocol.PathList)
	if err != nil {
		return err
	}
	writeStdout(body)
	return nil
}

func runOpen(httpClient *client.Client, args []string) error {
	if containsHelp(args) {
		printHelp("open")
		return nil
	}

	parsed := parse(args, 0)
	req := protocol.OpenRequest{
		Alias:            parsed.flags["alias"],
		JDBCURL:          parsed.flags["jdbc-url"],
		User:             parsed.flags["user"],
		PasswordKeychain: parsed.flags["password-keychain"],
		ReadOnly:         hasFlag(parsed.flags, "read-only"),
	}

	if req.Alias == "" {
		return fmt.Errorf("--alias required")
	}
	if req.JDBCURL == "" {
		return fmt.Errorf("--jdbc-url required")
	}
	if hasFlag(parsed.flags, "password-stdin") {
		password, err := readPasswordStdin()
		if err != nil {
			return err
		}
		req.Password = password
	}

	body, err := httpClient.PostJSON(protocol.PathOpen, req)
	if err != nil {
		return err
	}
	writeStdout(body)
	return nil
}

func runAliasCommand(httpClient *client.Client, args []string, path string, help string) error {
	if containsHelp(args) {
		printHelp(help)
		return nil
	}

	parsed := parse(args, 0)
	alias := parsed.flags["alias"]
	if alias == "" {
		return fmt.Errorf("--alias required")
	}

	body, err := httpClient.PostJSON(path, protocol.AliasRequest{Alias: alias})
	if err != nil {
		return err
	}
	writeStdout(body)
	return nil
}

func runQuery(httpClient *client.Client, args []string) error {
	if containsHelp(args) {
		printHelp("query")
		return nil
	}

	parsed := parse(args, 0)
	alias := parsed.flags["alias"]
	if alias == "" {
		return fmt.Errorf("--alias required")
	}

	sqlText, err := sqlArgument(parsed.flags, parsed.positionals)
	if err != nil {
		return err
	}

	body, err := httpClient.PostJSON(protocol.PathQuery, protocol.SQLRequest{
		Alias: alias,
		SQL:   sqlText,
		JSON:  hasFlag(parsed.flags, "json"),
	})
	if err != nil {
		return err
	}
	writeStdout(body)
	return nil
}

func runExec(httpClient *client.Client, args []string) error {
	if containsHelp(args) {
		printHelp("exec")
		return nil
	}

	parsed := parse(args, 0)
	alias := parsed.flags["alias"]
	if alias == "" {
		return fmt.Errorf("--alias required")
	}

	sqlText, err := sqlArgument(parsed.flags, parsed.positionals)
	if err != nil {
		return err
	}

	body, err := httpClient.PostJSON(protocol.PathExec, protocol.SQLRequest{
		Alias: alias,
		SQL:   sqlText,
	})
	if err != nil {
		return err
	}
	writeStdout(body)
	return nil
}

func runDescribe(httpClient *client.Client, args []string) error {
	if containsHelp(args) {
		printHelp("describe")
		return nil
	}

	parsed := parse(args, 0)
	req := protocol.DescribeRequest{
		Alias: parsed.flags["alias"],
		Table: parsed.flags["table"],
	}
	if req.Alias == "" {
		return fmt.Errorf("--alias required")
	}
	if req.Table == "" {
		return fmt.Errorf("--table required")
	}

	body, err := httpClient.PostJSON(protocol.PathDescribe, req)
	if err != nil {
		return err
	}
	writeStdout(body)
	return nil
}

func runBatch(httpClient *client.Client, args []string) error {
	if containsHelp(args) {
		printHelp("batch")
		return nil
	}

	parsed := parse(args, 0)
	defaultAlias := parsed.flags["alias"]

	body, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	if defaultAlias != "" {
		body, err = injectDefaultAlias(body, defaultAlias)
		if err != nil {
			return err
		}
	}

	response, err := httpClient.PostText(protocol.PathBatch, "application/x-ndjson", body)
	if err != nil {
		return err
	}
	writeStdout(response)
	return nil
}

func parse(args []string, start int) parsedArgs {
	flags := make(map[string]string)
	positionals := make([]string, 0)

	for i := start; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") {
			key := strings.TrimPrefix(arg, "--")
			_, isBool := boolFlags[key]
			if isBool || i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				flags[key] = ""
				continue
			}
			flags[key] = args[i+1]
			i++
			continue
		}
		positionals = append(positionals, arg)
	}

	return parsedArgs{
		flags:       flags,
		positionals: positionals,
	}
}

func sqlArgument(flags map[string]string, positionals []string) (string, error) {
	if len(positionals) > 0 {
		return strings.Join(positionals, " "), nil
	}
	if sqlText := flags["sql"]; strings.TrimSpace(sqlText) != "" {
		return sqlText, nil
	}
	return "", fmt.Errorf("SQL argument required")
}

func hasFlag(flags map[string]string, key string) bool {
	_, ok := flags[key]
	return ok
}

func printTopHelp() {
	_, _ = io.WriteString(os.Stdout, topHelp)
}

func printHelp(cmd string) {
	text := helpText(cmd)
	_, _ = io.WriteString(os.Stdout, text)
	if !strings.HasSuffix(text, "\n") {
		_, _ = io.WriteString(os.Stdout, "\n")
	}
}

func helpText(cmd string) string {
	switch cmd {
	case "ping":
		return `ping  Check if the daemon is running.

Usage:
  jdbc-cli ping
`
	case "list":
		return `list  List all open connection aliases.

Usage:
  jdbc-cli list
`
	case "open":
		return `open  Open a new JDBC connection and register it under an alias.

Usage:
  jdbc-cli open --alias <name> --jdbc-url <url> [auth flags]

Flags:
  --alias <name>               Alias to register this connection under (required)
  --jdbc-url <url>             JDBC URL, e.g. jdbc:mysql://localhost:3306/mydb (required)
  --user <user>                Database username
  --password-stdin             Read password from stdin
  --password-keychain <ref>    Look up password from macOS Keychain (service/account)
  --read-only                  Block write operations (exec, begin, commit, rollback) on this alias
`
	case "close":
		return `close  Close an open connection.

Usage:
  jdbc-cli close --alias <name>

Flags:
  --alias <name>    Connection alias to close (required)
`
	case "query":
		return `query  Run a SELECT query and print results.

Usage:
  jdbc-cli query --alias <name> [--json] '<SQL>'

Flags:
  --alias <name>    Connection alias (required)
  --json            Output results as JSON array instead of TSV

Example:
  jdbc-cli query --alias mydb 'SELECT * FROM users LIMIT 10'
  jdbc-cli query --alias mydb --json 'SELECT id, name FROM users'
`
	case "exec":
		return `exec  Run a write SQL statement (INSERT, UPDATE, DELETE, DDL).

Usage:
  jdbc-cli exec --alias <name> '<SQL>'

Flags:
  --alias <name>    Connection alias (required)

Output:
  {"rowsAffected": <n>}

Note: blocked if the alias was opened with --read-only.
`
	case "schema":
		return `schema  List all tables in the database.

Usage:
  jdbc-cli schema --alias <name>

Flags:
  --alias <name>    Connection alias (required)
`
	case "describe":
		return `describe  Show column definitions for a table.

Usage:
  jdbc-cli describe --alias <name> --table <table>

Flags:
  --alias <name>     Connection alias (required)
  --table <table>    Table name (required)

Output:
  JSON array of {name, type, size, nullable} objects.
`
	case "begin":
		return `begin  Begin a transaction on the connection.

Usage:
  jdbc-cli begin --alias <name>

Flags:
  --alias <name>    Connection alias (required)

Note: blocked if the alias was opened with --read-only.
`
	case "commit":
		return `commit  Commit the current transaction.

Usage:
  jdbc-cli commit --alias <name>

Flags:
  --alias <name>    Connection alias (required)

Note: blocked if the alias was opened with --read-only.
`
	case "rollback":
		return `rollback  Roll back the current transaction.

Usage:
  jdbc-cli rollback --alias <name>

Flags:
  --alias <name>    Connection alias (required)

Note: blocked if the alias was opened with --read-only.
`
	case "batch":
		return `batch  Execute multiple operations from stdin in NDJSON format.

Usage:
  echo '<ndjson>' | jdbc-cli batch [--alias <default-alias>]

Flags:
  --alias <name>    Default alias injected into ops that omit it (optional)

Each stdin line is a JSON object with an "op" field:
  {"op":"query","alias":"mydb","sql":"SELECT 1","json":false}
  {"op":"exec","alias":"mydb","sql":"INSERT INTO t VALUES (1)"}
  {"op":"begin","alias":"mydb"}
  {"op":"commit","alias":"mydb"}
  {"op":"rollback","alias":"mydb"}

Output is one NDJSON result line per input line.

Note: blocked if the alias was opened with --read-only.
`
	default:
		return fmt.Sprintf("No help available for %q.\n", cmd)
	}
}

func injectDefaultAlias(body []byte, alias string) ([]byte, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 1024), 10*1024*1024)

	var out bytes.Buffer
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		updated, err := injectAlias(line, alias)
		if err != nil {
			return nil, err
		}
		out.WriteString(updated)
		out.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func injectAlias(line string, alias string) (string, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		return line, nil
	}
	if _, ok := obj["alias"]; ok {
		return line, nil
	}

	value, err := json.Marshal(alias)
	if err != nil {
		return "", err
	}
	obj["alias"] = value

	updated, err := json.Marshal(obj)
	if err != nil {
		return "", err
	}
	return string(updated), nil
}

func readPasswordStdin() (string, error) {
	body, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(body), "\r\n"), nil
}

func containsHelp(args []string) bool {
	for _, arg := range args {
		if isHelp(arg) {
			return true
		}
	}
	return false
}

func isHelp(arg string) bool {
	return arg == "--help" || arg == "-h"
}

func writeStdout(body []byte) {
	if len(body) == 0 {
		return
	}
	_, _ = os.Stdout.Write(body)
	if body[len(body)-1] != '\n' {
		_, _ = os.Stdout.Write([]byte("\n"))
	}
}
