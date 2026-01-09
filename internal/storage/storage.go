// Package storage provides a factory for creating record repositories based on DSN.
package storage

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/honeybbq/tsdns"
	redisrepo "github.com/honeybbq/tsdns/repository/redis"
	sqlrepo "github.com/honeybbq/tsdns/repository/sql"
)

var (
	errDSNRequired          = errors.New("storage dsn is required")
	errSQLitePathMissing    = errors.New("sqlite dsn missing path")
	errPostgresURLRequired  = errors.New("postgres dsn must be url-style (postgres://...)")
	errMySQLHostMissing     = errors.New("mysql dsn missing host")
	errMySQLDBMissing       = errors.New("mysql dsn missing database name")
	errMySQLUserMissing     = errors.New("mysql dsn missing username")
	errRedisUserUnsupported = errors.New("redis username is not supported (use password-only auth)")
	errRedisHostMissing     = errors.New("redis dsn missing host")
	errRedisDBInvalid       = errors.New("invalid redis db number")
	errUnsupportedBackend   = errors.New("unsupported storage backend")
	errSchemeRequired       = errors.New("storage dsn must include a scheme")
	errUnsupportedScheme    = errors.New("unsupported storage scheme")
)

// Backend represents the type of storage backend.
type Backend string

const (
	// BackendSQL represents a SQL-based storage backend.
	BackendSQL Backend = "sql"
	// BackendRedis represents a Redis-based storage backend.
	BackendRedis Backend = "redis"

	memoryDSN = ":memory:"
)

type parsedDSN struct {
	backend Backend

	// backend-specific data
	sqlDialect sqlrepo.Dialect
	sqlDSN     string
	redisOpt   redisrepo.Options
}

// Open creates a new RecordRepository based on the provided DSN string.
func Open(dsn string) (tsdns.RecordRepository, Backend, error) {
	p, err := parseDSN(dsn)
	if err != nil {
		return nil, "", err
	}

	switch p.backend {
	case BackendSQL:
		repo, err := sqlrepo.NewRepository(sqlrepo.Options{
			Dialect: p.sqlDialect,
			DSN:     p.sqlDSN,
		})

		return repo, p.backend, err
	case BackendRedis:
		repo, err := redisrepo.NewRepository(p.redisOpt)

		return repo, p.backend, err
	default:
		return nil, "", fmt.Errorf("%w: %q", errUnsupportedBackend, p.backend)
	}
}

// Detect returns the backend type for the given DSN without opening a connection.
func Detect(dsn string) (Backend, error) {
	p, err := parseDSN(dsn)
	if err != nil {
		return "", err
	}

	return p.backend, nil
}

func parseDSN(dsn string) (parsedDSN, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return parsedDSN{}, errDSNRequired
	}

	u, err := url.Parse(dsn)
	if err != nil {
		return parsedDSN{}, fmt.Errorf("invalid storage dsn: %w", err)
	}
	if u.Scheme == "" {
		return parsedDSN{}, fmt.Errorf("%w (got %q)", errSchemeRequired, dsn)
	}

	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "sqlite":
		return parseSQLiteDSN(u)
	case "postgres", "postgresql":
		return parsePostgresDSN(u, dsn)
	case "mysql", "mariadb":
		return parseMySQLDSN(u)
	case "redis":
		return parseRedisDSN(u)
	default:
		return parsedDSN{}, fmt.Errorf("%w: %q", errUnsupportedScheme, u.Scheme)
	}
}

func parseSQLiteDSN(u *url.URL) (parsedDSN, error) {
	d := strings.TrimSpace(sqliteDSNFromURL(u))
	if d == "" {
		return parsedDSN{}, errSQLitePathMissing
	}

	return parsedDSN{backend: BackendSQL, sqlDialect: sqlrepo.DialectSQLite, sqlDSN: d}, nil
}

func parsePostgresDSN(u *url.URL, dsn string) (parsedDSN, error) {
	// Require URL-style DSN (postgres:// or postgresql://) to keep things predictable.
	if u.Host == "" {
		return parsedDSN{}, errPostgresURLRequired
	}

	return parsedDSN{backend: BackendSQL, sqlDialect: sqlrepo.DialectPostgres, sqlDSN: dsn}, nil
}

func parseMySQLDSN(u *url.URL) (parsedDSN, error) {
	mysqlDSN, err := mysqlDSNFromURL(u)
	if err != nil {
		return parsedDSN{}, err
	}

	return parsedDSN{backend: BackendSQL, sqlDialect: sqlrepo.DialectMySQL, sqlDSN: mysqlDSN}, nil
}

func parseRedisDSN(u *url.URL) (parsedDSN, error) {
	opt, err := parseRedisURL(u)
	if err != nil {
		return parsedDSN{}, err
	}

	return parsedDSN{backend: BackendRedis, redisOpt: opt}, nil
}

func sqliteDSNFromURL(u *url.URL) string {
	var base string
	if u.Opaque != "" {
		base = u.Opaque
	} else {
		base = u.Path
	}

	// Support sqlite:///:memory:
	if base == "/:memory:" {
		base = memoryDSN
	}

	// Preserve query parameters as part of the driver DSN.
	if u.RawQuery != "" {
		if strings.Contains(base, "?") {
			base += "&" + u.RawQuery
		} else {
			base += "?" + u.RawQuery
		}
	}

	return base
}

func mysqlDSNFromURL(u *url.URL) (string, error) {
	// Support two forms:
	// 1) URL style: mysql://user:pass@host:3306/db?parseTime=true
	// 2) Opaque (go-sql-driver style): mysql:user:pass@tcp(host:3306)/db?parseTime=true
	if u.Opaque != "" {
		return u.Opaque, nil
	}

	if u.Host == "" {
		return "", errMySQLHostMissing
	}
	dbName := strings.TrimPrefix(u.Path, "/")
	if strings.TrimSpace(dbName) == "" {
		return "", errMySQLDBMissing
	}

	user, pass := extractUserInfo(u.User)
	if user == "" {
		return "", errMySQLUserMissing
	}

	hostPort := ensureMySQLPort(u.Host)
	rawQuery := buildMySQLQuery(u.Query())

	creds := user
	if pass != "" {
		creds = user + ":" + pass
	}
	dsn := fmt.Sprintf("%s@tcp(%s)/%s", creds, hostPort, dbName)
	if rawQuery != "" {
		dsn += "?" + rawQuery
	}

	return dsn, nil
}

func extractUserInfo(u *url.Userinfo) (string, string) {
	if u == nil {
		return "", ""
	}
	user := u.Username()
	pass, _ := u.Password()

	return user, pass
}

func ensureMySQLPort(host string) string {
	_, _, err := net.SplitHostPort(host)
	if err != nil {
		if !strings.Contains(host, ":") {
			return host + ":3306"
		}
	}

	return host
}

func buildMySQLQuery(q url.Values) string {
	// Ensure parseTime=true for correct time scanning.
	if q.Get("parseTime") == "" {
		q.Set("parseTime", "true")
	}

	return q.Encode()
}

func parseRedisURL(u *url.URL) (redisrepo.Options, error) {
	if !strings.EqualFold(u.Scheme, "redis") {
		return redisrepo.Options{}, fmt.Errorf("%w (redis): %q", errUnsupportedScheme, u.Scheme)
	}

	if u.User != nil && u.User.Username() != "" {
		// Our repository does not support ACL username yet.
		return redisrepo.Options{}, errRedisUserUnsupported
	}

	addr := ensureRedisPort(u.Host)
	if addr == "" {
		return redisrepo.Options{}, errRedisHostMissing
	}

	_, password := extractUserInfo(u.User)
	db, err := extractRedisDB(u.Path)
	if err != nil {
		return redisrepo.Options{}, err
	}

	prefix := strings.TrimSpace(u.Query().Get("prefix"))

	return redisrepo.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
		Prefix:   prefix,
	}, nil
}

func ensureRedisPort(host string) string {
	if host == "" {
		return ""
	}
	_, _, err := net.SplitHostPort(host)
	if err != nil {
		if !strings.Contains(host, ":") {
			return host + ":6379"
		}
	}

	return host
}

func extractRedisDB(path string) (int, error) {
	dbStr := strings.Trim(path, "/")
	if dbStr == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(dbStr)
	if err != nil || n < 0 {
		return 0, errRedisDBInvalid
	}

	return n, nil
}
