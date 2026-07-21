package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/honeybbq/tsdns/core"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// RecordManager defines the interface for managing TSDNS records via the API.
type RecordManager interface {
	ListRecords(ctx context.Context) ([]*tsdns.Record, error)
	GetRecord(ctx context.Context, domain string) (*tsdns.Record, error)
	UpsertRecord(ctx context.Context, record *tsdns.Record) error
	DeleteRecord(ctx context.Context, domain string) error
	DeleteInstanceRecords(ctx context.Context, instanceID int64) error
}

// Server represents the admin HTTP API server.
type Server struct {
	record     RecordManager
	httpServer *http.Server
	addr       string
	socket     string
	token      string
}

// New creates a new admin API server with the specified configuration.
func New(addr, socket, token string, record RecordManager) *Server {
	s := &Server{
		addr:   addr,
		socket: socket,
		token:  token,
		record: record,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.Handle("GET /metrics", promhttp.Handler())

	mux.HandleFunc("GET /api/v1/records", s.handleRecordsList)
	mux.HandleFunc("POST /api/v1/records", s.handleRecordsUpsert)

	mux.HandleFunc("GET /api/v1/records/{domain}", s.handleRecordGet)
	mux.HandleFunc("DELETE /api/v1/records/{domain}", s.handleRecordDelete)

	// Catch invalid record paths (e.g. encoded slashes) and return a 400 instead of 404.
	mux.HandleFunc("GET /api/v1/records/{rest...}", s.handleRecordPathInvalid)
	mux.HandleFunc("DELETE /api/v1/records/{rest...}", s.handleRecordPathInvalid)

	mux.HandleFunc("DELETE /api/v1/instances/{id}/records", s.handleInstanceRecordsDelete)

	handler := http.Handler(mux)
	handler = s.authMiddleware(handler)

	const readHeaderTimeout = 5 * time.Second
	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	return s
}

// ListenAndServe starts the HTTP API server on both TCP and Unix socket if configured.
func (s *Server) ListenAndServe() error {
	if strings.TrimSpace(s.addr) == "" && strings.TrimSpace(s.socket) == "" {
		return http.ErrServerClosed
	}

	const errChSize = 2
	errCh := make(chan error, errChSize)

	// Listen on TCP
	if strings.TrimSpace(s.addr) != "" {
		go func() {
			errCh <- s.httpServer.ListenAndServe()
		}()
	}

	// Listen on Unix Socket
	if strings.TrimSpace(s.socket) != "" {
		// Remove existing socket file if it exists
		_ = os.Remove(s.socket)

		lc := net.ListenConfig{}
		listener, err := lc.Listen(context.Background(), "unix", s.socket)
		if err != nil {
			return fmt.Errorf("listen unix socket: %w", err)
		}

		// Ensure the socket is world-writable so any user can use the CLI locally.
		// For stricter environments, users should manage the permissions themselves.
		const socketPerm = 0o666
		/* #nosec G302 */
		err = os.Chmod(s.socket, socketPerm)
		if err != nil {
			return fmt.Errorf("chmod unix socket: %w", err)
		}

		go func() {
			errCh <- s.httpServer.Serve(listener)
		}()
	}

	return <-errCh
}

// Shutdown gracefully shuts down the HTTP API server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
