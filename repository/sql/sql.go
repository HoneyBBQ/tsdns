// Package sql provides a SQL-based implementation of the record repository.
package sql

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	sqlite "github.com/glebarez/sqlite"
	"github.com/honeybbq/tsdns"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	errDialectRequired = errors.New("sql dialect is required")
	errDSNRequired     = errors.New("sql dsn is required")
	errRecordNotFound  = errors.New("record not found")
	errRecordNil       = errors.New("record is nil")
	errDomainRequired  = errors.New("domain is required")
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
	db *gorm.DB
}

// recordModel defines the database schema for a single TSDNS target.
type recordModel struct {
	CreatedAt  time.Time `gorm:"not null"`
	UpdatedAt  time.Time `gorm:"not null"`
	Domain     string    `gorm:"size:255;not null;index"`
	IP         string    `gorm:"not null;type:text"`
	ID         int64     `gorm:"primaryKey;autoIncrement"`
	InstanceID int64     `gorm:"not null;index"`
	Priority   int       `gorm:"not null;default:0"`
	Port       uint16    `gorm:"not null;default:0"`
}

// TableName returns the table name for the record model.
func (recordModel) TableName() string { return "record" }

// NewRepository creates a new RecordRepository backed by a SQL database.
func NewRepository(opt Options) (tsdns.RecordRepository, error) {
	if strings.TrimSpace(string(opt.Dialect)) == "" {
		return nil, errDialectRequired
	}
	if strings.TrimSpace(opt.DSN) == "" {
		return nil, errDSNRequired
	}

	dialector, err := buildDialector(opt)
	if err != nil {
		return nil, err
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}

	// Optimize IP column for PostgreSQL using the 'inet' type.
	if opt.Dialect == DialectPostgres {
		_ = db.Exec("ALTER TABLE record ALTER COLUMN ip TYPE inet USING ip::inet")
	}

	r := &repository{db: db}

	// Auto-migrate the database schema.
	err = r.db.AutoMigrate(&recordModel{})
	if err != nil {
		return nil, err
	}

	return r, nil
}

func buildDialector(opt Options) (gorm.Dialector, error) {
	switch opt.Dialect {
	case DialectSQLite:
		if opt.DSN != ":memory:" {
			dir := filepath.Dir(opt.DSN)
			if dir != "." && dir != "" {
				const dirPerm = 0o750
				// Use 0o750 for better security (G301).
				err := os.MkdirAll(dir, dirPerm)
				if err != nil {
					return nil, fmt.Errorf("create sqlite directory %q: %w", dir, err)
				}
			}
		}

		return sqlite.Open(opt.DSN), nil
	case DialectPostgres:
		return postgres.Open(opt.DSN), nil
	case DialectMySQL:
		return mysql.Open(opt.DSN), nil
	default:
		return nil, fmt.Errorf("%w: %q", errUnsupportedDialect, opt.Dialect)
	}
}

var errUnsupportedDialect = errors.New("unsupported sql dialect")

// Find retrieves all records from the database, grouped by domain.
func (r *repository) Find(ctx context.Context) ([]*tsdns.Record, error) {
	var models []recordModel
	err := r.db.WithContext(ctx).Order("domain, priority").Find(&models).Error
	if err != nil {
		return nil, err
	}

	return groupModels(models), nil
}

// FindByDomain retrieves all targets for a single domain name.
func (r *repository) FindByDomain(ctx context.Context, domain string) (*tsdns.Record, error) {
	var models []recordModel
	err := r.db.WithContext(ctx).Where("domain = ?", domain).Order("priority").Find(&models).Error
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

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		createdAt := now

		// Preserve CreatedAt timestamp from existing record if it exists.
		var existing recordModel
		err := tx.Where("domain = ?", record.Domain).First(&existing).Error
		if err == nil {
			createdAt = existing.CreatedAt
		}

		// Delete existing targets for this domain before inserting new ones.
		err = tx.Where("domain = ?", record.Domain).Delete(&recordModel{}).Error
		if err != nil {
			return err
		}

		if len(record.Targets) == 0 {
			// Save a special row with empty IP to represent NORESPONSE.
			m := recordModel{
				InstanceID: record.InstanceID,
				Domain:     record.Domain,
				IP:         "",
				Port:       0,
				Priority:   0,
				CreatedAt:  createdAt,
				UpdatedAt:  now,
			}

			return tx.Create(&m).Error
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
			err = tx.Create(&m).Error
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
				ID:         m.ID, // Use ID of the first target row
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
	res := r.db.WithContext(ctx).Where("domain = ?", domain).Delete(&recordModel{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errRecordNotFound
	}

	return nil
}

// DeleteByInstanceID removes all records associated with the specified instance ID.
func (r *repository) DeleteByInstanceID(ctx context.Context, instanceID int64) error {
	return r.db.WithContext(ctx).Where("instance_id = ?", instanceID).Delete(&recordModel{}).Error
}

// Close closes the underlying database connection.
func (r *repository) Close() error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}

	return sqlDB.Close()
}
