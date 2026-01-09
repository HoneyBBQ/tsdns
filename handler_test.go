package tsdns

import (
	"io"
	"log/slog"
	"net"
	"net/netip"
	"testing"
	"time"
)

var nopLogger = slog.New(slog.DiscardHandler)

func readAllWithTimeout(t *testing.T, conn net.Conn) string {
	t.Helper()
	timeout := 250 * time.Millisecond

	type result struct {
		err error
		b   []byte
	}

	ch := make(chan result, 1)
	go func() {
		b, err := io.ReadAll(conn)
		ch <- result{b: b, err: err}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("read failed: %v", r.err)
		}

		return string(r.b)
	case <-time.After(timeout):
		_ = conn.Close()
		t.Fatalf("timeout reading from connection after %s", timeout)

		return ""
	}
}

func TestServer_handleQuery_hitWithPort(t *testing.T) {
	s := &Server{
		cache: map[string]*Record{
			"demo.example.com": {
				Domain:  "demo.example.com",
				Targets: []netip.AddrPort{netip.AddrPortFrom(netip.MustParseAddr("1.2.3.4"), 9987)},
			},
		},
		wildcardRecords: nil,
		logger:          nopLogger,
		metrics:         NewNopMetrics(),
	}

	serverConn, clientConn := net.Pipe()
	defer func() { _ = clientConn.Close() }()

	done := make(chan struct{})
	go func() {
		s.handleQuery(serverConn)
		close(done)
	}()

	_, err := clientConn.Write([]byte("  demo.example.com\r\n"))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	got := readAllWithTimeout(t, clientConn)
	if got != "1.2.3.4:9987" {
		t.Fatalf("unexpected response: %q", got)
	}

	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("server did not close the connection in time")
	}
}

func TestServer_handleQuery_hitWithoutPort(t *testing.T) {
	s := &Server{
		cache: map[string]*Record{
			"demo.example.com": {
				Domain:  "demo.example.com",
				Targets: []netip.AddrPort{netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), 0)},
			},
		},
		wildcardRecords: nil,
		logger:          nopLogger,
		metrics:         NewNopMetrics(),
	}

	serverConn, clientConn := net.Pipe()
	defer func() { _ = clientConn.Close() }()

	go s.handleQuery(serverConn)
	_, _ = clientConn.Write([]byte("demo.example.com\n"))

	got := readAllWithTimeout(t, clientConn)
	if got != "127.0.0.1" {
		t.Fatalf("unexpected response: %q", got)
	}
}

func TestServer_handleQuery_miss(t *testing.T) {
	s := &Server{
		cache:           map[string]*Record{},
		wildcardRecords: nil,
		logger:          nopLogger,
		metrics:         NewNopMetrics(),
	}

	serverConn, clientConn := net.Pipe()
	defer func() { _ = clientConn.Close() }()

	go s.handleQuery(serverConn)
	_, _ = clientConn.Write([]byte("unknown.example.com\n"))

	got := readAllWithTimeout(t, clientConn)
	if got != "404\n" {
		t.Fatalf("unexpected response: %q", got)
	}
}

func TestServer_handleQuery_emptyInput(t *testing.T) {
	s := &Server{
		cache:           map[string]*Record{},
		wildcardRecords: nil,
		logger:          nopLogger,
		metrics:         NewNopMetrics(),
	}

	serverConn, clientConn := net.Pipe()
	defer func() { _ = clientConn.Close() }()

	go s.handleQuery(serverConn)
	_, _ = clientConn.Write([]byte("\n"))

	got := readAllWithTimeout(t, clientConn)
	if got != "" {
		t.Fatalf("expected empty response, got: %q", got)
	}
}

func TestServer_handleQuery_wildcardAndDefaultAndNoResponse(t *testing.T) {
	s := &Server{
		cache: map[string]*Record{
			"voice.example.com": {Domain: "voice.example.com", Targets: nil},
		},
		wildcardRecords: []*Record{
			{Domain: "*.example.com", Targets: []netip.AddrPort{netip.AddrPortFrom(netip.MustParseAddr("1.2.3.4"), 0)}},
			{Domain: "*", Targets: []netip.AddrPort{netip.AddrPortFrom(netip.MustParseAddr("9.9.9.9"), 0)}},
		},
		logger:  nopLogger,
		metrics: NewNopMetrics(),
	}

	// Wildcard: should match suffix and substitute $PORT.
	{
		serverConn, clientConn := net.Pipe()
		defer func() { _ = clientConn.Close() }()

		go s.handleQuery(serverConn)
		_, _ = clientConn.Write([]byte("foo.example.com:12000\n"))

		got := readAllWithTimeout(t, clientConn)
		if got != "1.2.3.4:12000" {
			t.Fatalf("unexpected wildcard response: %q", got)
		}
	}

	// Default: should match anything and substitute $PORT.
	{
		serverConn, clientConn := net.Pipe()
		defer func() { _ = clientConn.Close() }()

		go s.handleQuery(serverConn)
		_, _ = clientConn.Write([]byte("unknown.tld:9987\n"))

		got := readAllWithTimeout(t, clientConn)
		if got != "9.9.9.9:9987" {
			t.Fatalf("unexpected default response: %q", got)
		}
	}

	// NORESPONSE: should not write anything.
	{
		serverConn, clientConn := net.Pipe()
		defer func() { _ = clientConn.Close() }()

		go s.handleQuery(serverConn)
		_, _ = clientConn.Write([]byte("voice.example.com:9987\n"))

		got := readAllWithTimeout(t, clientConn)
		if got != "" {
			t.Fatalf("expected no response, got: %q", got)
		}
	}
}
