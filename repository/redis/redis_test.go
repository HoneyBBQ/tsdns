package redis

import (
	"context"
	"net/netip"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/honeybbq/tsdns"
)

func TestRepository_CRUDAndInstanceIndex(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)

	repo, err := NewRepository(Options{
		Addr:   mr.Addr(),
		DB:     0,
		Prefix: "test",
	})
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() { _ = repo.Close() }()

	testRepositoryCreate(ctx, t, repo)
	testRepositoryUpsert(ctx, t, repo)
	testRepositoryDeleteByInstance(ctx, t, repo)
}

func testRepositoryCreate(ctx context.Context, t *testing.T, repo tsdns.RecordRepository) {
	t.Helper()
	// Create
	err := repo.Create(ctx, &tsdns.Record{
		InstanceID: 1,
		Domain:     "demo.example.com",
		Targets:    []netip.AddrPort{netip.AddrPortFrom(netip.MustParseAddr("1.2.3.4"), 9987)},
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	rec, err := repo.FindByDomain(ctx, "demo.example.com")
	if err != nil {
		t.Fatalf("FindByDomain failed: %v", err)
	}
	if len(rec.Targets) != 1 || rec.Targets[0].String() != "1.2.3.4:9987" || rec.InstanceID != 1 {
		t.Fatalf("unexpected record: %+v", rec)
	}

	list, err := repo.Find(ctx)
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 record, got %d", len(list))
	}
}

func testRepositoryUpsert(ctx context.Context, t *testing.T, repo tsdns.RecordRepository) {
	t.Helper()
	// Upsert with instance change should migrate the per-instance index.
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
}

func testRepositoryDeleteByInstance(ctx context.Context, t *testing.T, repo tsdns.RecordRepository) {
	t.Helper()
	// Deleting instance 1 should not delete the record anymore.
	err := repo.DeleteByInstanceID(ctx, 1)
	if err != nil {
		t.Fatalf("DeleteByInstanceID(1) failed: %v", err)
	}
	_, err = repo.FindByDomain(ctx, "demo.example.com")
	if err != nil {
		t.Fatalf("expected record to remain after DeleteByInstanceID(1), got %v", err)
	}

	// Deleting instance 2 should delete it.
	err = repo.DeleteByInstanceID(ctx, 2)
	if err != nil {
		t.Fatalf("DeleteByInstanceID(2) failed: %v", err)
	}
	_, err = repo.FindByDomain(ctx, "demo.example.com")
	if err == nil {
		t.Fatalf("expected record to be deleted after DeleteByInstanceID(2)")
	}
}

func TestRepository_Delete_missing(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)

	repo, err := NewRepository(Options{
		Addr:   mr.Addr(),
		DB:     0,
		Prefix: "test",
	})
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() { _ = repo.Close() }()

	err = repo.Delete(ctx, "nope.example.com")
	if err == nil {
		t.Fatalf("expected Delete to fail for missing record")
	}
}

func TestRepository_Create_validation(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)

	repo, err := NewRepository(Options{
		Addr:   mr.Addr(),
		DB:     0,
		Prefix: "test",
	})
	if err != nil {
		t.Fatalf("NewRepository failed: %v", err)
	}
	defer func() { _ = repo.Close() }()

	err = repo.Create(ctx, &tsdns.Record{Targets: []netip.AddrPort{netip.AddrPortFrom(netip.MustParseAddr("1.2.3.4"), 0)}})
	if err == nil {
		t.Fatalf("expected error for missing domain")
	}
}
