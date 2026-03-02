// Package sql provides a SQL-based implementation of the record repository.
package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/honeybbq/tsdns"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/mysqldialect"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/extra/bunslog"


	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)

var (
	errDialectRequired    = errors.New("sql dialect is required")
	errDSNRequired        = errors.New("sql dsn is required")
	errRecordNotFound     = errors.New("record not found")
	errRecordNil          = errors.New("record is nil")
	errDomainRequired     = errors.New("domain is required")
	errUnsupportedDialect = errors.New("unsupported sql dialect")
)

// Dialect represents the type of SQL database.
type Dialect string

const (
	// DialectSQLite represents the SQLite database dialect.
	DialectSQLite Dialect = "sqlite"
	// DialectPostgres represents the PostgreSQL database dialect.
	DialectPostgres Dialect = "postgres"
	// DialectMySQL represents the MySQL database dialect.
	DialectMySQL Dialect = "mysql"
)

// Options defines configuration parameters for the SQL repository.
type Options struct {
	Dialect Dialect
	DSN     string
}

type repository struct {
	db *bun.DB
}

type recordModel struct {
	bun.BaseModel `bun:"table:record"`

	CreatedAt  time.Time `bun:"created_at,notnull"`
	UpdatedAt  time.Time `bun:"updated_at,notnull"`
	Domain     string    `bun:"domain,notnull"`
	IP         string    `bun:"ip,notnull,type:text"`
	ID         int64     `bun:"id,pk,autoincrement"`
	InstanceID int64     `bun:"instance_id,notnull"`
	Priority   int       `bun:"priority,notnull,default:0"`
	Port       uint16    `bun:"port,notnull,default:0"`
}

// NewRepository creates a new RecordRepository backed by a SQL database.
func NewRepository(opt Options) (tsdns.RecordRepository, error) {
	if strings.TrimSpace(string(opt.Dialect)) == "" {
		return nil, errDialectRequired
	}
	if strings.TrimSpace(opt.DSN) == "" {
		return nil, errDSNRequired
	}

	db, err := openDB(opt)
	if err != nil {
		return nil, err
	}

	const slowQueryThreshold = 3 * time.Second
	db.AddQueryHook(bunslog.NewQueryHook(
		bunslog.WithQueryLogLevel(slog.LevelDebug),
		bunslog.WithSlowQueryLogLevel(slog.LevelWarn),
		bunslog.WithErrorQueryLogLevel(slog.LevelError),
		bunslog.WithSlowQueryThreshold(slowQueryThreshold),
	))

	ctx := context.Background()

	err = migrateSchema(ctx, db)
	if err != nil {
		return nil, err
	}

	// Create indexes for frequently queried columns.
	_, _ = db.NewCreateIndex().Model((*recordModel)(nil)).
		Index("idx_record_domain").Column("domain").IfNotExists().Exec(ctx)
	_, _ = db.NewCreateIndex().Model((*recordModel)(nil)).
		Index("idx_record_instance_id").Column("instance_id").IfNotExists().Exec(ctx)

	// Optimize IP column for PostgreSQL using the 'inet' type.
	if opt.Dialect == DialectPostgres {
		_, _ = db.ExecContext(ctx, "ALTER TABLE record ALTER COLUMN ip TYPE inet USING ip::inet")
	}

	return &repository{db: db}, nil
}

func migrateSchema(ctx context.Context, db *bun.DB) error {
	_, err := db.NewCreateTable().Model((*recordModel)(nil)).IfNotExists().Exec(ctx)

	return err
}

func openDB(opt Options) (*bun.DB, error) {
	switch opt.Dialect {
	case DialectSQLite:
		return openSQLite(opt.DSN)
	case DialectPostgres:
		return openPostgres(opt.DSN), nil
	case DialectMySQL:
		return openMySQL(opt.DSN)
	default:
		return nil, fmt.Errorf("%w: %q", errUnsupportedDialect, opt.Dialect)
	}
}

func openSQLite(dsn string) (*bun.DB, error) {
	if dsn != ":memory:" {
		dir := filepath.Dir(dsn)
		if dir != "." && dir != "" {
			const dirPerm = 0o750
			err := os.MkdirAll(dir, dirPerm)
			if err != nil {
				return nil, fmt.Errorf("create sqlite directory %q: %w", dir, err)
			}
		}
	}

	sqldb, err := sql.Open("sqlite", applySQLitePragmas(dsn))
	if err != nil {
		return nil, err
	}

	return bun.NewDB(sqldb, sqlitedialect.New()), nil
}

// applySQLitePragmas appends performance-tuned PRAGMA settings to the DSN.
func applySQLitePragmas(dsn string) string {
	pragmas := []string{
		"_pragma=journal_mode(WAL)",
		"_pragma=synchronous(NORMAL)",
		"_pragma=busy_timeout(5000)",
		"_pragma=cache_size(-64000)",
		"_pragma=foreign_keys(ON)",
	}

	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}

	return dsn + sep + strings.Join(pragmas, "&")
}

func openPostgres(dsn string) *bun.DB {
	connector := pgdriver.NewConnector(pgdriver.WithDSN(dsn))

	return bun.NewDB(sql.OpenDB(connector), pgdialect.New())
}

func openMySQL(dsn string) (*bun.DB, error) {
	sqldb, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	return bun.NewDB(sqldb, mysqldialect.New()), nil
}

// Find retrieves all records from the database, grouped by domain.
func (r *repository) Find(ctx context.Context) ([]*tsdns.Record, error) {
	var models []recordModel

	err := r.db.NewSelect().Model(&models).OrderExpr("domain, priority").Scan(ctx)
	if err != nil {
		return nil, err
	}

	return groupModels(models), nil
}

// FindByDomain retrieves all targets for a single domain name.
func (r *repository) FindByDomain(ctx context.Context, domain string) (*tsdns.Record, error) {
	var models []recordModel

	err := r.db.NewSelect().Model(&models).
		Where("domain = ?", domain).OrderExpr("priority").Scan(ctx)
	if err != nil {
		return nil, err
	}

	if len(models) == 0 {
		return nil, errRecordNotFound
	}

	return groupModels(models)[0], nil
}

// Create persists a new record (and its targets) or updates an existing one.
func (r *repository) Create(ctx context.Context, record *tsdns.Record) error {
	if record == nil {
		return errRecordNil
	}
	if strings.TrimSpace(record.Domain) == "" {
		return errDomainRequired
	}

	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		now := time.Now().UTC()
		createdAt := now

		// Preserve CreatedAt timestamp from existing record if it exists.
		var existing recordModel
		err := tx.NewSelect().Model(&existing).Where("domain = ?", record.Domain).Limit(1).Scan(ctx)
		if err == nil {
			createdAt = existing.CreatedAt
		}

		// Delete existing targets for this domain before inserting new ones.
		_, err = tx.NewDelete().Model((*recordModel)(nil)).Where("domain = ?", record.Domain).Exec(ctx)
		if err != nil {
			return err
		}

		if len(record.Targets) == 0 {
			m := recordModel{
				InstanceID: record.InstanceID,
				Domain:     record.Domain,
				IP:         "",
				Port:       0,
				Priority:   0,
				CreatedAt:  createdAt,
				UpdatedAt:  now,
			}
			_, err = tx.NewInsert().Model(&m).Exec(ctx)

			return err
		}

		for i, tp := range record.Targets {
			m := recordModel{
				InstanceID: record.InstanceID,
				Domain:     record.Domain,
				IP:         tp.Addr().String(),
				Port:       tp.Port(),
				Priority:   i,
				CreatedAt:  createdAt,
				UpdatedAt:  now,
			}

			_, err = tx.NewInsert().Model(&m).Exec(ctx)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

// groupModels merges multiple recordModel rows into a single tsdns.Record per domain.
func groupModels(models []recordModel) []*tsdns.Record {
	if len(models) == 0 {
		return nil
	}

	res := make([]*tsdns.Record, 0)
	groups := make(map[string]*tsdns.Record)
	order := make([]string, 0)

	for _, m := range models {
		if _, ok := groups[m.Domain]; !ok {
			r := &tsdns.Record{
				ID:         m.ID,
				InstanceID: m.InstanceID,
				Domain:     m.Domain,
				Targets:    make([]netip.AddrPort, 0),
				CreatedAt:  m.CreatedAt,
				UpdatedAt:  m.UpdatedAt,
			}
			groups[m.Domain] = r
			order = append(order, m.Domain)
		}

		if m.IP != "" {
			addr, err := netip.ParseAddr(m.IP)
			if err == nil {
				groups[m.Domain].Targets = append(groups[m.Domain].Targets, netip.AddrPortFrom(addr, m.Port))
			}
		}
	}

	for _, d := range order {
		res = append(res, groups[d])
	}

	return res
}

// Delete removes all targets associated with the specified domain name.
func (r *repository) Delete(ctx context.Context, domain string) error {
	res, err := r.db.NewDelete().Model((*recordModel)(nil)).Where("domain = ?", domain).Exec(ctx)
	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if n == 0 {
		return errRecordNotFound
	}

	return nil
}

// DeleteByInstanceID removes all records associated with the specified instance ID.
func (r *repository) DeleteByInstanceID(ctx context.Context, instanceID int64) error {
	_, err := r.db.NewDelete().Model((*recordModel)(nil)).Where("instance_id = ?", instanceID).Exec(ctx)

	return err
}

// Close closes the underlying database connection.
func (r *repository) Close() error {
	return r.db.Close()
}
