package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/honeybbq/tsdns/core"
	"github.com/honeybbq/tsdns/internal/config"
	"github.com/honeybbq/tsdns/internal/storage"
	"github.com/spf13/cobra"
)

var (
	errRecordNil         = errors.New("record is nil")
	errDomainRequired    = errors.New("domain is required")
	errInstanceIDInvalid = errors.New("instance-id must be >= 0")
	errAPIRequired       = errors.New("api url or socket is required")
	errAPIError          = errors.New("api error")
)

type recordsOptions struct {
	configPath *string

	apiURL string
	token  string

	direct bool
}

// newRecordsCommand creates the 'records' command to manage TSDNS records.
func newRecordsCommand(configPath *string) *cobra.Command {
	opts := &recordsOptions{configPath: configPath}

	cmd := &cobra.Command{
		Use:   "records",
		Short: "Manage records",
	}

	cmd.PersistentFlags().StringVar(&opts.apiURL, "api-url", "", "Admin API base URL (e.g. http://127.0.0.1:8080)")
	cmd.PersistentFlags().StringVar(&opts.token, "token", "", "Admin API token (or use TSDNS_API_TOKEN)")
	cmd.PersistentFlags().BoolVar(&opts.direct, "direct", false, "Write to storage directly (bypass admin API)")

	cmd.AddCommand(
		newRecordsAddCommand(opts),
		newRecordsDeleteCommand(opts),
		newRecordsListCommand(opts),
	)

	return cmd
}

// newRecordsAddCommand creates the 'add' subcommand to create or update a record.
func newRecordsAddCommand(opts *recordsOptions) *cobra.Command {
	var (
		domain     string
		targetStr  string
		port       int32
		instanceID int64
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add or update a record",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runAddRecord(opts, domain, targetStr, port, instanceID)
		},
	}

	cmd.Flags().StringVar(&domain, "domain", "", "Domain to match (exact match)")
	cmd.Flags().StringVar(&targetStr, "target", "", "Target address (IP/hostname)")
	cmd.Flags().Int32Var(&port, "port", 0, "Target port (0 to omit)")
	cmd.Flags().Int64Var(&instanceID, "instance-id", 0, "Instance ID (default: 0)")

	_ = cmd.MarkFlagRequired("domain")
	_ = cmd.MarkFlagRequired("target")

	return cmd
}

func runAddRecord(opts *recordsOptions, domain, targetStr string,
	port int32, instanceID int64) error {
	cfg, err := config.Load(*opts.configPath)
	if err != nil {
		return err
	}

	var targets []netip.AddrPort
	if targetStr != "" && !strings.EqualFold(targetStr, "NORESPONSE") {
		parts := strings.FieldsSeq(strings.ReplaceAll(targetStr, ",", " "))
		for p := range parts {
			tp, errParse := netip.ParseAddrPort(p)
			if errParse == nil {
				targets = append(targets, tp)
			} else {
				addr, errParseAddr := netip.ParseAddr(strings.Trim(p, "[] "))
				if errParseAddr != nil {
					return fmt.Errorf("invalid target %q: %w", p, errParseAddr)
				}

				// Safe conversion because port is validated/input via Int32Var.
				// We assume it's in uint16 range for TSDNS.
				/* #nosec G115 */
				uPort := uint16(port)
				targets = append(targets, netip.AddrPortFrom(addr, uPort))
			}
		}
	}

	rec := &tsdns.Record{
		InstanceID: instanceID,
		Domain:     domain,
		Targets:    targets,
	}

	err = validateRecordInput(rec)
	if err != nil {
		return err
	}

	if opts.direct {
		return upsertRecordDirect(&cfg, rec)
	}

	client := resolveAPIClient(&cfg, opts.apiURL, opts.token)

	return upsertRecordViaAPI(client, rec)
}

// newRecordsDeleteCommand creates the 'delete' subcommand to remove a record.
func newRecordsDeleteCommand(opts *recordsOptions) *cobra.Command {
	var domain string

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a record",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load(*opts.configPath)
			if err != nil {
				return err
			}
			if strings.TrimSpace(domain) == "" {
				return errDomainRequired
			}

			if opts.direct {
				return deleteRecordDirect(&cfg, domain)
			}

			client := resolveAPIClient(&cfg, opts.apiURL, opts.token)

			return deleteRecordViaAPI(client, domain)
		},
	}

	cmd.Flags().StringVar(&domain, "domain", "", "Domain to delete")
	_ = cmd.MarkFlagRequired("domain")

	return cmd
}

// newRecordsListCommand creates the 'list' subcommand to retrieve all records.
func newRecordsListCommand(opts *recordsOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List records",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(*opts.configPath)
			if err != nil {
				return err
			}

			if opts.direct {
				recs, errList := listRecordsDirect(&cfg)
				if errList != nil {
					return errList
				}

				return printJSON(cmd, recs)
			}

			client := resolveAPIClient(&cfg, opts.apiURL, opts.token)
			recs, errList := listRecordsViaAPI(client)
			if errList != nil {
				return errList
			}

			return printJSON(cmd, recs)
		},
	}

	return cmd
}

func resolveAPIClient(cfg *config.Config, apiURLFlag, tokenFlag string) *apiClient {
	apiURL := strings.TrimSpace(apiURLFlag)
	if apiURL == "" {
		apiURL = strings.TrimSpace(cfg.API.URL)
	}

	token := strings.TrimSpace(tokenFlag)
	if token == "" {
		token = strings.TrimSpace(cfg.API.Token)
	}

	const defaultTimeout = 10 * time.Second

	// Priority 1: Use Unix Domain Socket if configured and exists on disk.
	if cfg.API.Socket != "" {
		_, err := os.Stat(cfg.API.Socket)
		if err == nil {
			return &apiClient{
				baseURL: "http://unix", // Dummy base URL for http.Client with custom transport
				token:   "",            // No token needed for local socket
				hc: &http.Client{
					Timeout: defaultTimeout,
					Transport: &http.Transport{
						DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
							d := net.Dialer{}

							return d.DialContext(ctx, "unix", cfg.API.Socket)
						},
					},
				},
			}
		}
	}

	// Priority 2: Use HTTP API
	return newAPIClient(apiURL, token)
}

func validateRecordInput(r *tsdns.Record) error {
	if r == nil {
		return errRecordNil
	}
	if strings.TrimSpace(r.Domain) == "" {
		return errDomainRequired
	}
	if r.InstanceID < 0 {
		return errInstanceIDInvalid
	}

	return nil
}

func upsertRecordDirect(cfg *config.Config, rec *tsdns.Record) error {
	repo, err := newRepoForDirect(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = repo.Close() }()

	return repo.Create(context.Background(), rec)
}

func deleteRecordDirect(cfg *config.Config, domain string) error {
	repo, err := newRepoForDirect(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = repo.Close() }()

	return repo.Delete(context.Background(), domain)
}

func listRecordsDirect(cfg *config.Config) ([]*tsdns.Record, error) {
	repo, err := newRepoForDirect(cfg)
	if err != nil {
		return nil, err
	}
	defer func() { _ = repo.Close() }()

	return repo.Find(context.Background())
}

func newRepoForDirect(cfg *config.Config) (tsdns.RecordRepository, error) {
	repo, _, err := storage.Open(cfg.Storage.DSN)

	return repo, err
}

type apiClient struct {
	hc      *http.Client
	baseURL string
	token   string
}

func newAPIClient(baseURL, token string) *apiClient {
	const defaultTimeout = 10 * time.Second

	return &apiClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		hc: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

func upsertRecordViaAPI(c *apiClient, rec *tsdns.Record) error {
	if c == nil || c.baseURL == "" {
		return errAPIRequired
	}

	targets := make([]string, 0, len(rec.Targets))
	for _, tp := range rec.Targets {
		targets = append(targets, tp.String())
	}

	payload := map[string]any{
		"instance_id": rec.InstanceID,
		"domain":      rec.Domain,
		"targets":     targets,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		c.baseURL+"/api/v1/records", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	setAuthHeaders(req, c.token)

	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return nil
	}

	return decodeAPIError(resp)
}

func deleteRecordViaAPI(c *apiClient, domain string) error {
	if c == nil || c.baseURL == "" {
		return errAPIRequired
	}

	escapedDomain := url.PathEscape(domain)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete,
		c.baseURL+"/api/v1/records/"+escapedDomain, http.NoBody)
	if err != nil {
		return err
	}
	setAuthHeaders(req, c.token)

	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return nil
	}

	return decodeAPIError(resp)
}

func listRecordsViaAPI(c *apiClient) ([]*tsdns.Record, error) {
	if c == nil || c.baseURL == "" {
		return nil, errAPIRequired
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		c.baseURL+"/api/v1/records", http.NoBody)
	if err != nil {
		return nil, err
	}
	setAuthHeaders(req, c.token)

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeAPIError(resp)
	}

	var out []*tsdns.Record
	err = json.NewDecoder(resp.Body).Decode(&out)
	if err != nil {
		return nil, err
	}

	return out, nil
}

func setAuthHeaders(req *http.Request, token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
}

func decodeAPIError(resp *http.Response) error {
	var body struct {
		Error string `json:"error"`
	}
	err := json.NewDecoder(resp.Body).Decode(&body)
	if err == nil && body.Error != "" {
		return fmt.Errorf("%w (%s): %s", errAPIError, resp.Status, body.Error)
	}

	return fmt.Errorf("%w: %s", errAPIError, resp.Status)
}

func printJSON(cmd *cobra.Command, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), string(b))

	return err
}
