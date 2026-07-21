package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"testing"

	"github.com/honeybbq/tsdns/core"
)

type memManager struct {
	records           map[string]*tsdns.Record
	mu                sync.Mutex
	getErrAfterUpsert bool
}

func newMemManager() *memManager {
	return &memManager{records: make(map[string]*tsdns.Record)}
}

var errNotFound = errors.New("not found")

func (m *memManager) ListRecords(_ context.Context) ([]*tsdns.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]*tsdns.Record, 0, len(m.records))
	for _, r := range m.records {
		out = append(out, r)
	}

	return out, nil
}

func (m *memManager) GetRecord(_ context.Context, domain string) (*tsdns.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	r, ok := m.records[domain]
	if !ok {
		return nil, errNotFound
	}

	return r, nil
}

func (m *memManager) UpsertRecord(_ context.Context, record *tsdns.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.getErrAfterUpsert {
		// Store it, but pretend GetRecord will fail later (used by a test).
		m.records[record.Domain] = record

		return nil
	}

	m.records[record.Domain] = record

	return nil
}

func (m *memManager) DeleteRecord(_ context.Context, domain string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.records, domain)

	return nil
}

func (m *memManager) DeleteInstanceRecords(_ context.Context, instanceID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for d, r := range m.records {
		if r.InstanceID == instanceID {
			delete(m.records, d)
		}
	}

	return nil
}

func TestServer_Auth(t *testing.T) {
	mgr := newMemManager()
	s := New("127.0.0.1:0", "", "secret", mgr)
	h := s.httpServer.Handler

	// healthz is always public
	{
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example/healthz", http.NoBody)
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
	}

	// records require auth
	{
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example/api/v1/records", http.NoBody)
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	}

	// Authorization: Bearer <token>
	{
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example/api/v1/records", http.NoBody)
		req.Header.Set("Authorization", "Bearer secret")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
	}

	// X-API-Token: <token>
	{
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example/api/v1/records", http.NoBody)
		req.Header.Set("X-Api-Token", "secret")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
	}
}

func TestServer_Records_CRUD(t *testing.T) {
	mgr := newMemManager()
	s := New("127.0.0.1:0", "", "", mgr) // no auth
	h := s.httpServer.Handler

	testServerRecordsCRUDPost(t, h)
	testServerRecordsCRUDGet(t, h)
	testServerRecordsCRUDDelete(t, h)
}

func testServerRecordsCRUDPost(t *testing.T, h http.Handler) {
	t.Helper()
	t.Run("POST invalid json", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://example/api/v1/records",
			bytes.NewBufferString("{"))
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rr.Code)
		}
	})

	t.Run("POST missing domain", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://example/api/v1/records",
			bytes.NewBufferString(`{"target":"1.2.3.4"}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rr.Code)
		}
	})

	t.Run("POST port out of range", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://example/api/v1/records",
			bytes.NewBufferString(`{"domain":"a","targets":["1.2.3.4"],"port":70000}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rr.Code)
		}
	})

	t.Run("POST success", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://example/api/v1/records",
			bytes.NewBufferString(`{"domain":"demo.example.com","targets":["1.2.3.4:9987"],"port":0,"instance_id":0}`))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d", rr.Code)
		}
	})
}

func testServerRecordsCRUDGet(t *testing.T, h http.Handler) {
	t.Helper()
	t.Run("GET list", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example/api/v1/records", http.NoBody)
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		var out []*tsdns.Record
		err := json.Unmarshal(rr.Body.Bytes(), &out)
		if err != nil {
			t.Fatalf("invalid json response: %v", err)
		}
		if len(out) != 1 || out[0].Domain != "demo.example.com" {
			t.Fatalf("unexpected list: %+v", out)
		}
	})

	t.Run("GET by domain", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
			"http://example/api/v1/records/demo.example.com", http.NoBody)
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("reject encoded slash", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
			"http://example/api/v1/records/foo%2Fbar", http.NoBody)
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rr.Code)
		}
	})
}

func testServerRecordsCRUDDelete(t *testing.T, h http.Handler) {
	t.Helper()
	t.Run("DELETE by domain", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete,
			"http://example/api/v1/records/demo.example.com", http.NoBody)
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", rr.Code)
		}
	})
}

func TestServer_DeleteInstanceRecords(t *testing.T) {
	mgr := newMemManager()
	mgr.records["a.example.com"] = &tsdns.Record{
		InstanceID: 1, Domain: "a.example.com",
		Targets: []netip.AddrPort{netip.AddrPortFrom(netip.MustParseAddr("1.1.1.1"), 0)},
	}
	mgr.records["b.example.com"] = &tsdns.Record{
		InstanceID: 2, Domain: "b.example.com",
		Targets: []netip.AddrPort{netip.AddrPortFrom(netip.MustParseAddr("2.2.2.2"), 0)},
	}

	s := New("127.0.0.1:0", "", "", mgr)
	h := s.httpServer.Handler

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodDelete,
		"http://example/api/v1/instances/1/records", http.NoBody)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}

	// a.example.com should be removed, b.example.com should remain.
	_, err := mgr.GetRecord(context.Background(), "a.example.com")
	if err == nil {
		t.Fatalf("expected a.example.com to be removed")
	}
	_, err = mgr.GetRecord(context.Background(), "b.example.com")
	if err != nil {
		t.Fatalf("expected b.example.com to remain, got %v", err)
	}
}
