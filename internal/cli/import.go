// Package cli provides the command-line interface for managing the TSDNS server.
package cli

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strings"

	"github.com/honeybbq/tsdns"
	"github.com/honeybbq/tsdns/internal/config"
	"github.com/honeybbq/tsdns/internal/importer/tsdnsini"
	"github.com/honeybbq/tsdns/internal/storage"
	"github.com/spf13/cobra"
)

var (
	errInputRequired   = errors.New("input file is required (--input)")
	errStorageRequired = errors.New("storage dsn is required (set storage.dsn or use --dsn)")
)

// newImportCommand creates the 'import' command to import records from external sources.
func newImportCommand(configPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import records from external sources",
	}

	cmd.AddCommand(newImportTSDNSIniCommand(configPath))

	return cmd
}

// newImportTSDNSIniCommand creates the 'tsdns-ini' subcommand to import TeamSpeak tsdns_settings.ini entries.
func newImportTSDNSIniCommand(configPath *string) *cobra.Command {
	var (
		inputPath  string
		dsn        string
		instanceID int64
		dryRun     bool
	)

	cmd := &cobra.Command{
		Use:   "tsdns-ini",
		Short: "Import TeamSpeak tsdns_settings.ini entries into the current storage",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runImport(cmd, configPath, inputPath, dsn, instanceID, dryRun)
		},
	}

	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)

	cmd.Flags().StringVar(&inputPath, "input", "", "Path to tsdns_settings.ini")
	cmd.Flags().StringVar(&dsn, "dsn", "", "Storage DSN (overrides config)")
	cmd.Flags().Int64Var(&instanceID, "instance-id", 0, "Instance ID for imported records")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Parse and report counts without writing records")
	_ = cmd.MarkFlagRequired("input")

	return cmd
}

func runImport(cmd *cobra.Command, configPath *string,
	inputPath, dsn string, instanceID int64, dryRun bool) error {
	if inputPath == "" {
		return errInputRequired
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if dsn == "" {
		dsn = cfg.Storage.DSN
	}
	if dsn == "" {
		return errStorageRequired
	}

	res, err := tsdnsini.ParseFile(inputPath)
	if err != nil {
		return err
	}

	if dryRun {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "parsed %d entries (skipped %d)\n",
			len(res.Entries), res.Skipped)

		return nil
	}

	repo, _, err := storage.Open(dsn)
	if err != nil {
		return err
	}
	defer func() { _ = repo.Close() }()

	count, err := importEntries(cmd.Context(), repo, res.Entries, instanceID)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "imported %d records (skipped %d)\n",
		count, res.Skipped)

	return nil
}

func importEntries(ctx context.Context, repo tsdns.RecordRepository,
	entries []tsdnsini.Entry, instanceID int64) (int, error) {
	imported := 0
	for _, e := range entries {
		var targets []netip.AddrPort
		if e.Value != "" && !strings.EqualFold(e.Value, "NORESPONSE") {
			parts := strings.FieldsSeq(strings.ReplaceAll(e.Value, ",", " "))
			for p := range parts {
				tp, err := netip.ParseAddrPort(p)
				if err == nil {
					targets = append(targets, tp)
				} else {
					addr, err := netip.ParseAddr(strings.Trim(p, "[] "))
					if err != nil {
						return 0, fmt.Errorf("line %d: invalid IP %q: %w", e.Line, p, err)
					}
					targets = append(targets, netip.AddrPortFrom(addr, 0)) // 0 means $PORT
				}
			}
		}
		rec := &tsdns.Record{
			InstanceID: instanceID,
			Domain:     e.Ident,
			Targets:    targets,
		}
		err := repo.Create(ctx, rec)
		if err != nil {
			return 0, fmt.Errorf("line %d: create record %q failed: %w", e.Line, e.Ident, err)
		}
		imported++
	}

	return imported, nil
}
