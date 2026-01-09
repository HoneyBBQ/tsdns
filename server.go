// Package tsdns provides a TeamSpeak TSDNS protocol compatible server.
package tsdns

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"regexp"
	"sync"
	"time"
)

var (
	errInvalidIP    = errors.New("invalid IP address")
	errRepoRequired = errors.New("repository is required")
)

// ServerBuilder represents a builder for TSDNS server.
type ServerBuilder struct {
	server *Server
	err    error
}

// compiledRecord associates a DNS record with its compiled regular expression.
type compiledRecord struct {
	*Record

	re *regexp.Regexp
}

// Server represents a TSDNS server instance.
type Server struct {
	repository           RecordRepository
	ctx                  context.Context
	metrics              Metrics
	listener             net.Listener
	logger               *slog.Logger
	cache                map[string]*Record
	cancel               context.CancelFunc
	addr                 string
	wildcardRecords      []*Record
	regexRecords         []*compiledRecord
	cacheRefreshInterval time.Duration
	mu                   sync.RWMutex
	listenerMu           sync.Mutex
}

// NewServer creates a new TSDNS server builder for the specified IP.
func NewServer(ip string) *ServerBuilder {
	// Validate IP address
	if ip != "0.0.0.0" && net.ParseIP(ip) == nil {
		return &ServerBuilder{err: errInvalidIP}
	}

	return NewServerAddr(net.JoinHostPort(ip, "41144"))
}

// NewServerAddr creates a new TSDNS server builder with an explicit listen address.
// The listenAddr should be in the form "host:port", e.g. "0.0.0.0:41144".
func NewServerAddr(listenAddr string) *ServerBuilder {
	ctx, cancel := context.WithCancel(context.Background())

	const defaultCacheRefreshInterval = 30 * time.Second
	builder := &ServerBuilder{
		server: &Server{
			addr:                 listenAddr,
			cache:                make(map[string]*Record),
			cacheRefreshInterval: defaultCacheRefreshInterval,
			ctx:                  ctx,
			cancel:               cancel,
			logger:               NewDefaultLogger(), // Default logger
			metrics:              NewNopMetrics(),    // Default to no-op
		},
	}

	_, err := net.ResolveTCPAddr("tcp", listenAddr)
	if err != nil {
		builder.err = fmt.Errorf("invalid listen address: %w", err)
	}

	return builder
}

// WithRepository sets the record repository for the server.
func (b *ServerBuilder) WithRepository(repo RecordRepository) *ServerBuilder {
	if b.err != nil {
		return b
	}
	b.server.repository = repo

	return b
}

// WithCacheRefreshInterval sets how often the in-memory cache is refreshed from the repository.
// Use 0 or a negative duration to disable periodic refresh.
func (b *ServerBuilder) WithCacheRefreshInterval(d time.Duration) *ServerBuilder {
	if b.err != nil {
		return b
	}
	b.server.cacheRefreshInterval = d

	return b
}

// WithLogger sets the logger for the server.
func (b *ServerBuilder) WithLogger(l *slog.Logger) *ServerBuilder {
	if b.err != nil {
		return b
	}
	b.server.logger = l

	return b
}

// WithMetrics sets the metrics implementation for the server.
func (b *ServerBuilder) WithMetrics(m Metrics) *ServerBuilder {
	if b.err != nil {
		return b
	}
	b.server.metrics = m

	return b
}

// Build creates and returns the server instance.
func (b *ServerBuilder) Build() (*Server, error) {
	if b.err != nil {
		return nil, b.err
	}

	// Validate required fields
	if b.server.repository == nil {
		return nil, errRepoRequired
	}

	return b.server, nil
}

// MustBuild creates and returns the server instance, panicking on error.
func (b *ServerBuilder) MustBuild() *Server {
	server, err := b.Build()
	if err != nil {
		panic(err)
	}

	return server
}

// Start initializes and runs the TSDNS server.
// It listens for incoming TCP connections and handles queries.
func (s *Server) Start() error {
	addr, err := net.ResolveTCPAddr("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("resolve address error: %w", err)
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen error: %w", err)
	}
	defer func() { _ = listener.Close() }()

	s.listenerMu.Lock()
	s.listener = listener
	s.listenerMu.Unlock()

	// Load cache initially
	err = s.loadCache()
	if err != nil {
		return fmt.Errorf("load cache error: %w", err)
	}

	// Start cache updater if periodic refresh is enabled
	if s.cacheRefreshInterval > 0 {
		go s.cacheUpdater()
	}

	s.logger.Info("TSDNS server started", slog.String("addr", s.addr))

	// Accept and handle incoming queries
	for {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			select {
			case <-s.ctx.Done():
				return nil
			default:
			}
			s.logger.Error("accept error", slog.Any("error", acceptErr))

			continue
		}

		go s.handleQuery(conn)
	}
}

// Close shuts down the server and releases associated resources.
func (s *Server) Close() error {
	s.logger.Info("Shutting down tsdns server")
	s.cancel()

	s.listenerMu.Lock()
	if s.listener != nil {
		_ = s.listener.Close()
		s.listener = nil
	}
	s.listenerMu.Unlock()

	return s.repository.Close()
}
