package sql

import (
	"context"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/honeybbq/tsdns/core"
)

func newTestRepo(t *testing.T) tsdns.RecordRepository {
	t.Helper()
	repo, err := NewRepository(Options{Dialect: DialectSQLite, DSN: ":memory:"})
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	return repo
}

func TestRepository_SQLiteMemory_CRUD(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	createdAt := testCreateAndFindByDomain(ctx, t, repo)
	testUpsertPreservesCreatedAt(ctx, t, repo, createdAt)
	testDelete(ctx, t, repo)
}

func testCreateAndFindByDomain(ctx context.Context, t *testing.T, repo tsdns.RecordRepository) time.Time {
	t.Helper()
	t.Run("Create", func(t *testing.T) {
		err := repo.Create(ctx, &tsdns.Record{
			InstanceID: 1,
			Domain:     "demo.example.com",
			Targets:    []netip.AddrPort{netip.AddrPortFrom(netip.MustParseAddr("1.2.3.4"), 9987)},
		})
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	})

	var createdAt time.Time
	t.Run("FindByDomain", func(t *testing.T) {
		rec, err := repo.FindByDomain(ctx, "demo.example.com")
		if err != nil {
			t.Fatalf("FindByDomain failed: %v", err)
		}
		if len(rec.Targets) != 1 || rec.Targets[0].String() != "1.2.3.4:9987" || rec.InstanceID != 1 {
			t.Fatalf("unexpected record: %+v", rec)
		}
		createdAt = rec.CreatedAt
	})

	return createdAt
}

func testUpsertPreservesCreatedAt(ctx context.Context, t *testing.T, repo tsdns.RecordRepository, createdAt time.Time) {
	t.Helper()
	t.Run("Upsert", func(t *testing.T) {
		err := repo.Create(ctx, &tsdns.Record{
			InstanceID: 2,
			Domain:     "demo.example.com",
			Targets:    []netip.AddrPort{netip.AddrPortFrom(netip.MustParseAddr("9.9.9.9"), 0)},
		})
		if err != nil {
			t.Fatalf("Create(upsert) failed: %v", err)
		}

		rec, err := repo.FindByDomain(ctx, "demo.example.com")
		if err != nil {
			t.Fatalf("FindByDomain(after upsert) failed: %v", err)
		}
		if rec.InstanceID != 2 || len(rec.Targets) != 1 || rec.Targets[0].String() != "9.9.9.9:0" {
			t.Fatalf("unexpected record after upsert: %+v", rec)
		}
		if !rec.CreatedAt.Equal(createdAt) {
			t.Fatalf("expected CreatedAt preserved, old=%v new=%v", createdAt, rec.CreatedAt)
		}
	})
}

func testDelete(ctx context.Context, t *testing.T, repo tsdns.RecordRepository) {
	t.Helper()
	t.Run("Delete", func(t *testing.T) {
		err := repo.Delete(ctx, "demo.example.com")
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
		_, err = repo.FindByDomain(ctx, "demo.example.com")
		if err == nil {
			t.Fatalf("expected record not found after delete")
		}
	})
}

func TestRepository_Find(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	// Empty repo returns nil.
	recs, err := repo.Find(ctx)
	if err != nil {
		t.Fatalf("Find(empty) failed: %v", err)
	}
	if recs != nil {
		t.Fatalf("expected nil for empty repo, got %d records", len(recs))
	}

	// Insert two domains, verify Find returns both grouped correctly.
	_ = repo.Create(ctx, &tsdns.Record{
		Domain:  "a.example.com",
		Targets: []netip.AddrPort{netip.AddrPortFrom(netip.MustParseAddr("1.1.1.1"), 80)},
	})
	_ = repo.Create(ctx, &tsdns.Record{
		Domain:  "b.example.com",
		Targets: []netip.AddrPort{netip.AddrPortFrom(netip.MustParseAddr("2.2.2.2"), 443)},
	})

	recs, err = repo.Find(ctx)
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 records, got %d", len(recs))
	}
}

func TestRepository_MultipleTargets(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	targets := []netip.AddrPort{
		netip.AddrPortFrom(netip.MustParseAddr("1.1.1.1"), 9987),
		netip.AddrPortFrom(netip.MustParseAddr("2.2.2.2"), 9988),
		netip.AddrPortFrom(netip.MustParseAddr("3.3.3.3"), 9989),
	}
	err := repo.Create(ctx, &tsdns.Record{Domain: "multi.example.com", Targets: targets})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	rec, err := repo.FindByDomain(ctx, "multi.example.com")
	if err != nil {
		t.Fatalf("FindByDomain failed: %v", err)
	}
	if len(rec.Targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(rec.Targets))
	}
	for i, want := range targets {
		if rec.Targets[i] != want {
			t.Fatalf("target[%d]: got %v, want %v", i, rec.Targets[i], want)
		}
	}
}

func TestRepository_EmptyTargets_NORESPONSE(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	err := repo.Create(ctx, &tsdns.Record{Domain: "silent.example.com", Targets: nil})
	if err != nil {
		t.Fatalf("Create(NORESPONSE) failed: %v", err)
	}

	rec, err := repo.FindByDomain(ctx, "silent.example.com")
	if err != nil {
		t.Fatalf("FindByDomain failed: %v", err)
	}
	if len(rec.Targets) != 0 {
		t.Fatalf("expected 0 targets (NORESPONSE), got %d", len(rec.Targets))
	}
}

func TestRepository_DeleteByInstanceID(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	_ = repo.Create(ctx, &tsdns.Record{
		InstanceID: 10, Domain: "a.example.com",
		Targets: []netip.AddrPort{netip.AddrPortFrom(netip.MustParseAddr("1.1.1.1"), 0)},
	})
	_ = repo.Create(ctx, &tsdns.Record{
		InstanceID: 20, Domain: "b.example.com",
		Targets: []netip.AddrPort{netip.AddrPortFrom(netip.MustParseAddr("2.2.2.2"), 0)},
	})

	err := repo.DeleteByInstanceID(ctx, 10)
	if err != nil {
		t.Fatalf("DeleteByInstanceID failed: %v", err)
	}

	_, err = repo.FindByDomain(ctx, "a.example.com")
	if err == nil {
		t.Fatal("expected a.example.com removed")
	}
	_, err = repo.FindByDomain(ctx, "b.example.com")
	if err != nil {
		t.Fatalf("expected b.example.com to remain: %v", err)
	}
}

func TestRepository_Delete_notFound(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	err := repo.Delete(ctx, "nope.example.com")
	if err == nil {
		t.Fatal("expected error deleting nonexistent domain")
	}
}

func TestRepository_FindByDomain_notFound(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	_, err := repo.FindByDomain(ctx, "nope.example.com")
	if err == nil {
		t.Fatal("expected error for nonexistent domain")
	}
}

func TestRepository_Create_validation(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	err := repo.Create(ctx, nil)
	if err == nil {
		t.Fatal("expected error for nil record")
	}

	err = repo.Create(ctx, &tsdns.Record{Domain: "  "})
	if err == nil {
		t.Fatal("expected error for blank domain")
	}
}

func TestNewRepository_invalidOptions(t *testing.T) {
	_, err := NewRepository(Options{})
	if err == nil {
		t.Fatal("expected error for empty options")
	}

	_, err = NewRepository(Options{Dialect: DialectSQLite})
	if err == nil {
		t.Fatal("expected error for empty DSN")
	}

	_, err = NewRepository(Options{Dialect: "bogus", DSN: ":memory:"})
	if err == nil {
		t.Fatal("expected error for unsupported dialect")
	}
}

// Integration tests for PostgreSQL and MySQL.
// Skipped unless the corresponding env var is set and not in short mode.

func TestRepository_Postgres_CRUD(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	dsn := os.Getenv("TSDNS_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TSDNS_TEST_POSTGRES_DSN not set")
	}

	repo, err := NewRepository(Options{Dialect: DialectPostgres, DSN: dsn})
	if err != nil {
		t.Fatalf("NewRepository(postgres) failed: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	ctx := context.Background()
	runRepositoryCRUDSuite(ctx, t, repo)
}

func TestRepository_MySQL_CRUD(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	dsn := os.Getenv("TSDNS_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TSDNS_TEST_MYSQL_DSN not set")
	}

	repo, err := NewRepository(Options{Dialect: DialectMySQL, DSN: dsn})
	if err != nil {
		t.Fatalf("NewRepository(mysql) failed: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	ctx := context.Background()
	runRepositoryCRUDSuite(ctx, t, repo)
}

// runRepositoryCRUDSuite exercises the full CRUD lifecycle against any backend.
func runRepositoryCRUDSuite(ctx context.Context, t *testing.T, repo tsdns.RecordRepository) {
	t.Helper()

	_ = repo.Delete(ctx, "integ.example.com")

	t.Run("Create+FindByDomain", func(t *testing.T) {
		integTestCreateAndFind(ctx, t, repo)
	})
	t.Run("Find", func(t *testing.T) {
		integTestFind(ctx, t, repo)
	})
	t.Run("Delete", func(t *testing.T) {
		integTestDelete(ctx, t, repo)
	})
}

func integTestCreateAndFind(ctx context.Context, t *testing.T, repo tsdns.RecordRepository) {
	t.Helper()
	err := repo.Create(ctx, &tsdns.Record{
		InstanceID: 1,
		Domain:     "integ.example.com",
		Targets:    []netip.AddrPort{netip.AddrPortFrom(netip.MustParseAddr("10.0.0.1"), 9987)},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	rec, err := repo.FindByDomain(ctx, "integ.example.com")
	if err != nil {
		t.Fatalf("FindByDomain: %v", err)
	}
	if len(rec.Targets) != 1 || rec.Targets[0].Addr().String() != "10.0.0.1" {
		t.Fatalf("unexpected record: %+v", rec)
	}
}

func integTestFind(ctx context.Context, t *testing.T, repo tsdns.RecordRepository) {
	t.Helper()
	recs, err := repo.Find(ctx)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}

	found := false
	for _, r := range recs {
		if r.Domain == "integ.example.com" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected integ.example.com in Find results")
	}
}

func integTestDelete(ctx context.Context, t *testing.T, repo tsdns.RecordRepository) {
	t.Helper()
	err := repo.Delete(ctx, "integ.example.com")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = repo.FindByDomain(ctx, "integ.example.com")
	if err == nil {
		t.Fatal("expected not found after delete")
	}
}
