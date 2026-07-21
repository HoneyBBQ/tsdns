package tsdns

import (
	"context"
	"errors"
	"net/netip"
	"time"
)

var errRecordNil = errors.New("record is nil")

// Record represents a single TSDNS record entry.
type Record struct {
	CreatedAt  time.Time        `json:"created_at"`
	UpdatedAt  time.Time        `json:"updated_at"`
	DeletedAt  *time.Time       `json:"deleted_at,omitempty"`
	Domain     string           `json:"domain"`
	Targets    []netip.AddrPort `json:"targets"`
	ID         int64            `json:"id"`
	InstanceID int64            `json:"instance_id"`
}

// RecordRepository defines the interface for persisting and retrieving TSDNS records.
type RecordRepository interface {
	Find(ctx context.Context) ([]*Record, error)
	FindByDomain(ctx context.Context, domain string) (*Record, error)
	Create(ctx context.Context, record *Record) error
	Delete(ctx context.Context, domain string) error
	DeleteByInstanceID(ctx context.Context, instanceID int64) error
	Close() error
}

// AddRecord adds a new DNS record to the system.
// It updates both the repository and the cache immediately.
func (s *Server) AddRecord(ctx context.Context, domain string, targets []netip.AddrPort) error {
	return s.AddRecordWithInstanceID(ctx, 0, domain, targets)
}

// AddRecordWithInstanceID adds (or updates) a DNS record for the given instance.
// It updates both the repository and the cache immediately.
func (s *Server) AddRecordWithInstanceID(ctx context.Context, instanceID int64,
	domain string, targets []netip.AddrPort) error {
	return s.UpsertRecord(ctx, &Record{
		InstanceID: instanceID,
		Domain:     domain,
		Targets:    targets,
	})
}

// UpsertRecord creates or updates a record in the repository and refreshes the in-memory cache.
func (s *Server) UpsertRecord(ctx context.Context, record *Record) error {
	if record == nil {
		return errRecordNil
	}

	timer := time.Now()
	err := s.repository.Create(ctx, record)
	if err != nil {
		s.metrics.RecordRepositoryOp("create", "error", time.Since(timer))

		return err
	}
	s.metrics.RecordRepositoryOp("create", "success", time.Since(timer))

	return s.loadCache()
}

// ListRecords returns all active (non-deleted) records.
func (s *Server) ListRecords(ctx context.Context) ([]*Record, error) {
	timer := time.Now()
	recs, err := s.repository.Find(ctx)
	if err != nil {
		s.metrics.RecordRepositoryOp("find", "error", time.Since(timer))

		return nil, err
	}
	s.metrics.RecordRepositoryOp("find", "success", time.Since(timer))

	return recs, nil
}

// GetRecord finds a record by domain name.
func (s *Server) GetRecord(ctx context.Context, domain string) (*Record, error) {
	timer := time.Now()
	rec, err := s.repository.FindByDomain(ctx, domain)
	if err != nil {
		s.metrics.RecordRepositoryOp("find_by_domain", "error", time.Since(timer))

		return nil, err
	}
	s.metrics.RecordRepositoryOp("find_by_domain", "success", time.Since(timer))

	return rec, nil
}

// RemoveRecord deletes a DNS record by domain name.
// It updates both the repository and the cache immediately.
func (s *Server) RemoveRecord(ctx context.Context, domain string) error {
	return s.DeleteRecord(ctx, domain)
}

// RemoveInstanceRecords removes all records associated with an instance.
// It updates both the repository and the cache immediately.
func (s *Server) RemoveInstanceRecords(ctx context.Context, instanceID int64) error {
	return s.DeleteInstanceRecords(ctx, instanceID)
}

// DeleteRecord deletes a record by domain name and refreshes the cache.
func (s *Server) DeleteRecord(ctx context.Context, domain string) error {
	timer := time.Now()
	err := s.repository.Delete(ctx, domain)
	if err != nil {
		s.metrics.RecordRepositoryOp("delete", "error", time.Since(timer))

		return err
	}
	s.metrics.RecordRepositoryOp("delete", "success", time.Since(timer))

	return s.loadCache()
}

// DeleteInstanceRecords removes all records for the instance and refreshes the cache.
func (s *Server) DeleteInstanceRecords(ctx context.Context, instanceID int64) error {
	timer := time.Now()
	err := s.repository.DeleteByInstanceID(ctx, instanceID)
	if err != nil {
		s.metrics.RecordRepositoryOp("delete_by_instance", "error", time.Since(timer))

		return err
	}
	s.metrics.RecordRepositoryOp("delete_by_instance", "success", time.Since(timer))

	return s.loadCache()
}
