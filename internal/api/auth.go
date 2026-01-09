// Package api provides the administration HTTP REST API for the TSDNS server.
package api

import (
	"net/http"
	"strings"
)

// authMiddleware validates the API token for incoming requests.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	if strings.TrimSpace(s.token) == "" {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Health endpoint is always public.
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)

			return
		}

		// Trust local connections via Unix Domain Socket.
		// In Go, requests from unix sockets have r.RemoteAddr as "@" or the socket path.
		// Since TCP addresses always contain a colon (e.g., "127.0.0.1:1234"),
		// we can detect UDS by checking for the absence of a colon.
		if !strings.Contains(r.RemoteAddr, ":") {
			next.ServeHTTP(w, r)

			return
		}

		if token := extractToken(r); token != s.token {
			writeError(w, http.StatusUnauthorized, "unauthorized")

			return
		}

		next.ServeHTTP(w, r)
	})
}

// extractToken extracts the API token from the request headers.
func extractToken(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-Api-Token")); v != "" {
		return v
	}

	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if auth == "" {
		return ""
	}
	const prefix = "Bearer "
	if after, ok := strings.CutPrefix(auth, prefix); ok {
		return strings.TrimSpace(after)
	}

	return ""
}
