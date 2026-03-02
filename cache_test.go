package tsdns

import (
	"net"
	"net/netip"
	"regexp"
	"testing"
)

func newTestServerWithRegex(t *testing.T, rec *Record, useRegex bool) *Server {
	t.Helper()

	s := &Server{
		cache:           make(map[string]*Record),
		wildcardRecords: nil,
		regexRecords:    []*compiledRecord{},
		logger:          nopLogger,
		metrics:         NewNopMetrics(),
	}

	if useRegex {
		s.tryProcessRegex(rec, &s.regexRecords)
	} else {
		s.tryProcessAdvancedGlob(rec, &s.regexRecords)
	}

	if len(s.regexRecords) != 1 {
		t.Fatalf("expected 1 compiled record, got %d", len(s.regexRecords))
	}

	return s
}

func assertQueryResponse(t *testing.T, s *Server, query, want string) {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	defer func() { _ = clientConn.Close() }()

	go s.handleQuery(serverConn)
	_, _ = clientConn.Write([]byte(query + "\n"))

	got := readAllWithTimeout(t, clientConn)
	if got != want {
		t.Errorf("query %q: got %q, want %q", query, got, want)
	}
}

func TestServer_handleQuery_regexPattern(t *testing.T) {
	target := netip.AddrPortFrom(netip.MustParseAddr("5.5.5.5"), 9987)
	rec := &Record{Domain: "reg:^voice\\d+\\.example\\.com$", Targets: []netip.AddrPort{target}}
	s := newTestServerWithRegex(t, rec, true)

	assertQueryResponse(t, s, "voice42.example.com", "5.5.5.5:9987")
	assertQueryResponse(t, s, "chat.example.com", "404\n")
}

func TestServer_handleQuery_advancedGlob(t *testing.T) {
	target := netip.AddrPortFrom(netip.MustParseAddr("6.6.6.6"), 0)
	rec := &Record{Domain: "voice-*.region-?.example.com", Targets: []netip.AddrPort{target}}
	s := newTestServerWithRegex(t, rec, false)

	assertQueryResponse(t, s, "voice-eu.region-1.example.com:9987", "6.6.6.6:9987")
	assertQueryResponse(t, s, "chat.example.com", "404\n")
}

func TestDomainToRegex(t *testing.T) {
	cases := []struct {
		domain string
		input  string
		match  bool
	}{
		{"*.example.com", "foo.example.com", true},
		{"*.example.com", "foo.other.com", false},
		{"voice-?.example.com", "voice-1.example.com", true},
		{"voice-?.example.com", "voice-12.example.com", false},
		{"*-eu.*.example.com", "voice-eu.region.example.com", true},
	}
	for _, tc := range cases {
		pattern := domainToRegex(tc.domain)
		re, err := regexp.Compile(pattern)
		if err != nil {
			t.Fatalf("domainToRegex(%q) produced invalid regex %q: %v", tc.domain, pattern, err)
		}
		got := re.MatchString(tc.input)
		if got != tc.match {
			t.Errorf("domainToRegex(%q).MatchString(%q) = %v, want %v (pattern=%q)",
				tc.domain, tc.input, got, tc.match, pattern)
		}
	}
}
