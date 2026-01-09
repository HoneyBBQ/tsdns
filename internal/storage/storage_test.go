package storage

import (
	"strings"
	"testing"

	sqlrepo "github.com/honeybbq/tsdns/repository/sql"
)

func TestParseDSN_sqlite(t *testing.T) {
	testParseDSNSqlitePath(t)
	testParseDSNSqliteMemory(t)
	testParseDSNSqliteParams(t)
}

func testParseDSNSqlitePath(t *testing.T) {
	t.Helper()
	t.Run("sqlite path", func(t *testing.T) {
		p, err := parseDSN("sqlite:///data/tsdns.sqlite")
		if err != nil {
			t.Fatalf("parseDSN failed: %v", err)
		}
		if p.backend != BackendSQL {
			t.Fatalf("expected backend=sql, got %q", p.backend)
		}
		if p.sqlDialect != sqlrepo.DialectSQLite {
			t.Fatalf("expected dialect=sqlite, got %q", p.sqlDialect)
		}
		if p.sqlDSN != "/data/tsdns.sqlite" {
			t.Fatalf("unexpected sqlite dsn: %q", p.sqlDSN)
		}
	})
}

func testParseDSNSqliteMemory(t *testing.T) {
	t.Helper()
	t.Run("sqlite memory", func(t *testing.T) {
		p, err := parseDSN("sqlite::memory:")
		if err != nil {
			t.Fatalf("parseDSN failed: %v", err)
		}
		if p.sqlDSN != ":memory:" {
			t.Fatalf("unexpected sqlite dsn: %q", p.sqlDSN)
		}
	})

	t.Run("sqlite memory url", func(t *testing.T) {
		p, err := parseDSN("sqlite:///:memory:")
		if err != nil {
			t.Fatalf("parseDSN failed: %v", err)
		}
		if p.sqlDSN != ":memory:" {
			t.Fatalf("unexpected sqlite dsn: %q", p.sqlDSN)
		}
	})
}

func testParseDSNSqliteParams(t *testing.T) {
	t.Helper()
	t.Run("sqlite with params", func(t *testing.T) {
		p, err := parseDSN("sqlite:///data/tsdns.sqlite?cache=shared&mode=rwc")
		if err != nil {
			t.Fatalf("parseDSN failed: %v", err)
		}
		if p.sqlDSN != "/data/tsdns.sqlite?cache=shared&mode=rwc" {
			t.Fatalf("unexpected sqlite dsn: %q", p.sqlDSN)
		}
	})
}

func TestParseDSN_postgres(t *testing.T) {
	p, err := parseDSN("postgres://user:pass@127.0.0.1:5432/db?sslmode=disable")
	if err != nil {
		t.Fatalf("parseDSN failed: %v", err)
	}
	if p.backend != BackendSQL {
		t.Fatalf("expected backend=sql, got %q", p.backend)
	}
	if p.sqlDialect != sqlrepo.DialectPostgres {
		t.Fatalf("expected dialect=postgres, got %q", p.sqlDialect)
	}
	if p.sqlDSN == "" {
		t.Fatalf("expected postgres dsn to be set")
	}
}

func TestParseDSN_mysql(t *testing.T) {
	p, err := parseDSN("mysql://user:pass@127.0.0.1:3306/db?charset=utf8mb4")
	if err != nil {
		t.Fatalf("parseDSN failed: %v", err)
	}
	if p.backend != BackendSQL {
		t.Fatalf("expected backend=sql, got %q", p.backend)
	}
	if p.sqlDialect != sqlrepo.DialectMySQL {
		t.Fatalf("expected dialect=mysql, got %q", p.sqlDialect)
	}
	if p.sqlDSN == "" || !strings.Contains(p.sqlDSN, "parseTime=true") {
		t.Fatalf("expected mysql dsn to include parseTime=true, got %q", p.sqlDSN)
	}
}

func TestParseDSN_redis(t *testing.T) {
	p, err := parseDSN("redis://:secret@127.0.0.1:6379/2?prefix=tsdns")
	if err != nil {
		t.Fatalf("parseDSN failed: %v", err)
	}
	if p.backend != BackendRedis {
		t.Fatalf("expected backend=redis, got %q", p.backend)
	}
	if p.redisOpt.Addr != "127.0.0.1:6379" {
		t.Fatalf("unexpected redis addr: %q", p.redisOpt.Addr)
	}
	if p.redisOpt.Password != "secret" {
		t.Fatalf("unexpected redis password: %q", p.redisOpt.Password)
	}
	if p.redisOpt.DB != 2 {
		t.Fatalf("unexpected redis db: %d", p.redisOpt.DB)
	}
	if p.redisOpt.Prefix != "tsdns" {
		t.Fatalf("unexpected redis prefix: %q", p.redisOpt.Prefix)
	}
}

func TestParseDSN_requiresScheme(t *testing.T) {
	_, err := parseDSN("/tmp/tsdns")
	if err == nil {
		t.Fatalf("expected error for DSN without scheme")
	}
}
