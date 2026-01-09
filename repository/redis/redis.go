// Package redis provides a Redis implementation of the record repository.
package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/honeybbq/tsdns"
)

var (
	errRedisAddrRequired = errors.New("redis addr is required")
	errRecordNotFound    = errors.New("record not found")
	errRecordNil         = errors.New("record is nil")
	errDomainRequired    = errors.New("domain is required")
)

// Options defines configuration parameters for the Redis repository.
type Options struct {
	Addr     string
	Password string
	Prefix   string
	DB       int
}

type repository struct {
	c      *goredis.Client
	prefix string
}

// NewRepository creates a new RecordRepository backed by Redis.
func NewRepository(opt Options) (tsdns.RecordRepository, error) {
	if strings.TrimSpace(opt.Addr) == "" {
		return nil, errRedisAddrRequired
	}
	if strings.TrimSpace(opt.Prefix) == "" {
		opt.Prefix = "tsdns"
	}

	c := goredis.NewClient(&goredis.Options{
		Addr:     opt.Addr,
		Password: opt.Password,
		DB:       opt.DB,
	})

	r := &repository{c: c, prefix: opt.Prefix}

	// Verify connectivity early to catch configuration errors at startup.
	err := c.Ping(context.Background()).Err()
	if err != nil {
		_ = c.Close()

		return nil, err
	}

	return r, nil
}

// Find retrieves all records from Redis using a pipeline for performance.
func (r *repository) Find(ctx context.Context) ([]*tsdns.Record, error) {
	domains, err := r.c.SMembers(ctx, r.keyDomains()).Result()
	if err != nil {
		return nil, err
	}
	if len(domains) == 0 {
		return []*tsdns.Record{}, nil
	}

	out := make([]*tsdns.Record, 0, len(domains))
	pipe := r.c.Pipeline()
	cmds := make([]*goredis.MapStringStringCmd, 0, len(domains))
	for _, d := range domains {
		cmds = append(cmds, pipe.HGetAll(ctx, r.keyRecord(d)))
	}
	_, err = pipe.Exec(ctx)
	if err != nil && !errors.Is(err, goredis.Nil) {
		return nil, err
	}

	for i, cmd := range cmds {
		m, err := cmd.Result()
		if err != nil {
			return nil, fmt.Errorf("hgetall record %q: %w", domains[i], err)
		}
		if len(m) == 0 {
			// Skip if the record is missing despite being in the domain set.
			continue
		}
		rec, err := decodeRecordMap(domains[i], m)
		if err != nil {
			return nil, fmt.Errorf("decode record %q: %w", domains[i], err)
		}
		out = append(out, rec)
	}

	return out, nil
}

// FindByDomain retrieves a single record by its domain name.
func (r *repository) FindByDomain(ctx context.Context, domain string) (*tsdns.Record, error) {
	m, err := r.c.HGetAll(ctx, r.keyRecord(domain)).Result()
	if err != nil {
		return nil, err
	}
	if len(m) == 0 {
		return nil, errRecordNotFound
	}

	return decodeRecordMap(domain, m)
}

// Create persists a new record or updates an existing one in Redis.
func (r *repository) Create(ctx context.Context, record *tsdns.Record) error {
	if record == nil {
		return errRecordNil
	}
	if strings.TrimSpace(record.Domain) == "" {
		return errDomainRequired
	}

	// Read existing instance_id to keep per-instance index consistent.
	var oldInstanceID int64
	v, err := r.c.HGet(ctx, r.keyRecord(record.Domain), "instance_id").Result()
	if err == nil {
		n, convErr := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if convErr == nil {
			oldInstanceID = n
		}
	}
	v, err = r.c.HGet(ctx, r.keyRecord(record.Domain), "created_at").Result()
	if err == nil && record.CreatedAt.IsZero() {
		tm, parseErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(v))
		if parseErr == nil {
			record.CreatedAt = tm
		}
	}

	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	record.UpdatedAt = now

	hash := encodeRecordMap(record)

	pipe := r.c.TxPipeline()
	pipe.HSet(ctx, r.keyRecord(record.Domain), hash)
	pipe.SAdd(ctx, r.keyDomains(), record.Domain)
	pipe.SAdd(ctx, r.keyInstanceDomains(record.InstanceID), record.Domain)
	if oldInstanceID != record.InstanceID {
		pipe.SRem(ctx, r.keyInstanceDomains(oldInstanceID), record.Domain)
	}
	_, err = pipe.Exec(ctx)

	return err
}

// Delete removes a record and its indices from Redis.
func (r *repository) Delete(ctx context.Context, domain string) error {
	v, err := r.c.HGet(ctx, r.keyRecord(domain), "instance_id").Result()
	if errors.Is(err, goredis.Nil) {
		return errRecordNotFound
	}
	if err != nil {
		return err
	}

	instanceID, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)

	pipe := r.c.TxPipeline()
	pipe.Del(ctx, r.keyRecord(domain))
	pipe.SRem(ctx, r.keyDomains(), domain)
	pipe.SRem(ctx, r.keyInstanceDomains(instanceID), domain)
	_, err = pipe.Exec(ctx)

	return err
}

// DeleteByInstanceID removes all records associated with the specified instance ID.
func (r *repository) DeleteByInstanceID(ctx context.Context, instanceID int64) error {
	domains, err := r.c.SMembers(ctx, r.keyInstanceDomains(instanceID)).Result()
	if err != nil {
		return err
	}

	if len(domains) == 0 {
		// Still delete the index key to avoid stale empty sets.
		return r.c.Del(ctx, r.keyInstanceDomains(instanceID)).Err()
	}

	pipe := r.c.TxPipeline()
	for _, d := range domains {
		pipe.Del(ctx, r.keyRecord(d))
		pipe.SRem(ctx, r.keyDomains(), d)
	}
	pipe.Del(ctx, r.keyInstanceDomains(instanceID))
	_, err = pipe.Exec(ctx)

	return err
}

// Close closes the underlying Redis client.
func (r *repository) Close() error {
	return r.c.Close()
}

func (r *repository) keyDomains() string {
	return r.prefix + ":domains"
}

func (r *repository) keyRecord(domain string) string {
	return r.prefix + ":record:" + domain
}

func (r *repository) keyInstanceDomains(instanceID int64) string {
	return fmt.Sprintf("%s:instance:%d:domains", r.prefix, instanceID)
}

// encodeRecordMap converts a Record to a map suitable for Redis HSET.
func encodeRecordMap(r *tsdns.Record) map[string]any {
	targets := make([]string, 0, len(r.Targets))
	for _, tp := range r.Targets {
		targets = append(targets, tp.String())
	}
	targetsJSON, err := json.Marshal(targets)
	if err != nil {
		// Should not happen with string slice
		targetsJSON = []byte("[]")
	}

	out := map[string]any{
		"instance_id": strconv.FormatInt(r.InstanceID, 10),
		"targets":     string(targetsJSON),
		"created_at":  r.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":  r.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}

	return out
}

// decodeRecordMap converts a Redis HGETALL map back to a Record.
func decodeRecordMap(domain string, m map[string]string) (*tsdns.Record, error) {
	var (
		instanceID int64
		createdAt  time.Time
		updatedAt  time.Time
		targets    []netip.AddrPort
	)

	if v := strings.TrimSpace(m["instance_id"]); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err == nil {
			instanceID = n
		}
	}
	if v := strings.TrimSpace(m["created_at"]); v != "" {
		tm, err := time.Parse(time.RFC3339Nano, v)
		if err == nil {
			createdAt = tm
		}
	}
	if v := strings.TrimSpace(m["updated_at"]); v != "" {
		tm, err := time.Parse(time.RFC3339Nano, v)
		if err == nil {
			updatedAt = tm
		}
	}

	if v := m["targets"]; v != "" {
		var targetStrings []string
		err := json.Unmarshal([]byte(v), &targetStrings)
		if err == nil {
			targets = parseTargetStrings(targetStrings)
		}
	}

	return &tsdns.Record{
		ID:         0,
		InstanceID: instanceID,
		Domain:     domain,
		Targets:    targets,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
		DeletedAt:  nil,
	}, nil
}

func parseTargetStrings(targetStrings []string) []netip.AddrPort {
	var targets []netip.AddrPort
	for _, s := range targetStrings {
		tp, err := netip.ParseAddrPort(s)
		if err == nil {
			targets = append(targets, tp)

			continue
		}

		addr, err := netip.ParseAddr(s)
		if err == nil {
			// Fallback for IP-only strings (port 0)
			targets = append(targets, netip.AddrPortFrom(addr, 0))
		}
	}

	return targets
}
