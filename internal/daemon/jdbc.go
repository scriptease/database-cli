package daemon

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/go-sql-driver/mysql"
)

type dialect string

const (
	dialectMySQL    dialect = "mysql"
	dialectPostgres dialect = "postgres"
	dialectSQLite   dialect = "sqlite"
)

type connectionConfig struct {
	DriverName       string
	DSN              string
	Dialect          dialect
	SingleConnection bool
}

func parseJDBCURL(jdbcURL string, user string, password string) (connectionConfig, error) {
	switch {
	case strings.HasPrefix(jdbcURL, "jdbc:mysql://"):
		return parseMySQLJDBC(jdbcURL, user, password)
	case strings.HasPrefix(jdbcURL, "jdbc:postgresql://"), strings.HasPrefix(jdbcURL, "jdbc:postgres://"):
		return parsePostgresJDBC(jdbcURL, user, password)
	case strings.HasPrefix(jdbcURL, "jdbc:sqlite:"):
		return parseSQLiteJDBC(jdbcURL)
	default:
		return connectionConfig{}, fmt.Errorf("unsupported jdbcUrl: %s", jdbcURL)
	}
}

func parseMySQLJDBC(jdbcURL string, user string, password string) (connectionConfig, error) {
	parsed, err := url.Parse(strings.TrimPrefix(jdbcURL, "jdbc:"))
	if err != nil {
		return connectionConfig{}, err
	}
	if parsed.Hostname() == "" {
		return connectionConfig{}, fmt.Errorf("mysql jdbcUrl is missing host")
	}

	cfg := mysql.NewConfig()
	cfg.Net = "tcp"
	port := parsed.Port()
	if port == "" {
		port = "3306"
	}
	cfg.Addr = net.JoinHostPort(parsed.Hostname(), port)
	cfg.DBName = strings.TrimPrefix(parsed.Path, "/")
	cfg.ParseTime = true
	cfg.Params = map[string]string{}

	if user != "" {
		cfg.User = user
	} else if parsed.User != nil {
		cfg.User = parsed.User.Username()
	}

	if password != "" {
		cfg.Passwd = password
	} else if parsed.User != nil {
		if parsedPassword, ok := parsed.User.Password(); ok {
			cfg.Passwd = parsedPassword
		}
	}

	for key, values := range parsed.Query() {
		if len(values) == 0 {
			continue
		}
		cfg.Params[key] = values[len(values)-1]
	}

	return connectionConfig{
		DriverName: "mysql",
		DSN:        cfg.FormatDSN(),
		Dialect:    dialectMySQL,
	}, nil
}

func parsePostgresJDBC(jdbcURL string, user string, password string) (connectionConfig, error) {
	parsed, err := url.Parse(strings.TrimPrefix(jdbcURL, "jdbc:"))
	if err != nil {
		return connectionConfig{}, err
	}
	if parsed.Hostname() == "" {
		return connectionConfig{}, fmt.Errorf("postgres jdbcUrl is missing host")
	}

	existingUser := ""
	existingPassword := ""
	if parsed.User != nil {
		existingUser = parsed.User.Username()
		existingPassword, _ = parsed.User.Password()
	}

	finalUser := existingUser
	if user != "" {
		finalUser = user
	}

	finalPassword := existingPassword
	if password != "" {
		finalPassword = password
	}

	if finalUser != "" {
		if finalPassword != "" {
			parsed.User = url.UserPassword(finalUser, finalPassword)
		} else {
			parsed.User = url.User(finalUser)
		}
	}

	return connectionConfig{
		DriverName: "pgx",
		DSN:        parsed.String(),
		Dialect:    dialectPostgres,
	}, nil
}

func parseSQLiteJDBC(jdbcURL string) (connectionConfig, error) {
	dsn := strings.TrimPrefix(jdbcURL, "jdbc:sqlite:")
	if dsn == "" {
		return connectionConfig{}, fmt.Errorf("sqlite jdbcUrl is missing database path")
	}
	if strings.HasPrefix(dsn, "//") {
		dsn = "/" + strings.TrimLeft(dsn, "/")
	}

	return connectionConfig{
		DriverName:       "sqlite",
		DSN:              dsn,
		Dialect:          dialectSQLite,
		SingleConnection: sqliteNeedsSingleConnection(dsn),
	}, nil
}

func sqliteNeedsSingleConnection(dsn string) bool {
	normalized := strings.ToLower(strings.TrimSpace(dsn))
	return normalized == ":memory:" || strings.Contains(normalized, "mode=memory")
}
