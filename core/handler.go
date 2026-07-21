package tsdns

import (
	"log/slog"
	"net"
	"strconv"
	"strings"
)

// handleQuery processes incoming DNS queries.
// It looks up the domain in the cache and returns the corresponding record.
// If no record is found, it returns "404".
func (s *Server) handleQuery(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	const maxQuerySize = 512
	buf := make([]byte, maxQuerySize)
	n, err := conn.Read(buf)
	if err != nil {
		return
	}

	raw := strings.TrimSpace(string(buf[:n]))
	if raw == "" {
		return
	}
	host, port := parseQuery(raw)
	s.logger.Debug("query received", slog.String("raw", raw))

	record, exists := s.lookupRecord(host)

	if !exists {
		s.handleNotFound(conn, host)

		return
	}

	s.handleHit(conn, host, port, record)
}

func (s *Server) handleNotFound(conn net.Conn, host string) {
	s.logger.Debug("record not found", slog.String("host", host))
	_, err := conn.Write([]byte("404\n"))
	if err != nil {
		s.logger.Error("write 404 error", slog.Any("error", err))
	}
	s.metrics.RecordQuery("miss")
}

func (s *Server) handleHit(conn net.Conn, host string, port int32, record *Record) {
	// Special case: NORESPONSE means we intentionally don't reply to this query.
	if len(record.Targets) == 0 {
		s.metrics.RecordQuery("hit")

		return
	}

	response := buildResponse(record, port)
	if response == "" {
		// No meaningful reply (e.g. $PORT requested but query port missing).
		s.metrics.RecordQuery("error")

		return
	}
	_, err := conn.Write([]byte(response))
	if err != nil {
		s.logger.Error("write response error", slog.Any("error", err))
		s.metrics.RecordQuery("error")

		return
	}
	s.logger.Debug("record found", slog.String("host", host), slog.String("response", response))
	s.metrics.RecordQuery("hit")
}

func (s *Server) lookupRecord(host string) (*Record, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 1. Exact match
	if rec, ok := s.cache[host]; ok {
		return rec, true
	}

	// 2. Regex match (includes advanced globs)
	for _, rec := range s.regexRecords {
		if rec.re.MatchString(host) {
			return rec.Record, true
		}
	}

	// 3. Traditional TSDNS wildcards (suffix match)
	// Wildcards are pre-sorted by domain length desc (most specific first).
	for _, rec := range s.wildcardRecords {
		pat := rec.Domain
		if !strings.HasPrefix(pat, "*") {
			continue
		}
		suffix := strings.TrimPrefix(pat, "*")
		if suffix == "" || strings.HasSuffix(host, suffix) {
			return rec, true
		}
	}

	return nil, false
}

func parseQuery(raw string) (string, int32) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", 0
	}

	// Support bracketed host: [::1]:9987 (mainly for parity with TS docs).
	if strings.HasPrefix(raw, "[") {
		if end := strings.Index(raw, "]"); end > 1 && end+1 < len(raw) && raw[end+1] == ':' {
			h := raw[1:end]
			p := raw[end+2:]
			// Use ParseUint with bitSize 16 to prevent overflow and ensure range.
			pn, err := strconv.ParseUint(p, 10, 16)
			if err == nil {
				return h, int32(pn)
			}
		}
	}

	// Typical: host:port (host is a DNS name; split on last ':').
	if idx := strings.LastIndex(raw, ":"); idx > 0 && idx+1 < len(raw) {
		h := raw[:idx]
		p := raw[idx+1:]
		// Use ParseUint with bitSize 16 to prevent overflow and ensure range.
		pn, err := strconv.ParseUint(p, 10, 16)
		if err == nil {
			return h, int32(pn)
		}
	}

	return raw, 0
}

func buildResponse(rec *Record, queryPort int32) string {
	// Special case: NORESPONSE.
	if len(rec.Targets) == 0 {
		return ""
	}

	var sb strings.Builder
	for i, tp := range rec.Targets {
		if i > 0 {
			sb.WriteByte(' ')
		}

		addr := tp.Addr()
		port := int32(tp.Port())

		// If port is 0, it means we should use the query's port ($PORT).
		// If query port is also missing, we return just the IP.
		usePort := true
		if port == 0 {
			if queryPort > 0 {
				port = queryPort
			} else {
				usePort = false
			}
		}

		if addr.Is6() {
			sb.WriteByte('[')
			sb.WriteString(addr.String())
			sb.WriteByte(']')
		} else {
			sb.WriteString(addr.String())
		}

		if usePort {
			sb.WriteByte(':')
			sb.WriteString(strconv.Itoa(int(port)))
		}
	}

	return sb.String()
}
