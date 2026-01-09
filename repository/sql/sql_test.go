package sql

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/honeybbq/tsdns"
)

func TestRepository_SQLiteMemory_CRUD(t *testing.T) {
	ctx := context.Background()
	repo, err := NewRepository(Options{
		Dialect: DialectSQLite,
		DSN:     ":memory:",
	})
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	createdAt := testRepositorySQLiteCreateAndFind(ctx, t, repo)
	testRepositorySQLiteUpsert(ctx, t, repo, createdAt)
	testRepositorySQLiteDelete(ctx, t, repo)
}

func testRepositorySQLiteCreateAndFind(ctx context.Context, t *testing.T, repo tsdns.RecordRepository) time.Time {
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

func testRepositorySQLiteUpsert(ctx context.Context, t *testing.T, repo tsdns.RecordRepository, createdAt time.Time) {
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

		rec2, err := repo.FindByDomain(ctx, "demo.example.com")
		if err != nil {
			t.Fatalf("FindByDomain(after upsert) failed: %v", err)
		}
		if rec2.InstanceID != 2 || len(rec2.Targets) != 1 || rec2.Targets[0].String() != "9.9.9.9:0" {
			t.Fatalf("unexpected record after upsert: %+v", rec2)
		}
		if !rec2.CreatedAt.Equal(createdAt) {
			t.Fatalf("expected CreatedAt preserved, old=%v new=%v", createdAt, rec2.CreatedAt)
		}
	})
}

func testRepositorySQLiteDelete(ctx context.Context, t *testing.T, repo tsdns.RecordRepository) {
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
