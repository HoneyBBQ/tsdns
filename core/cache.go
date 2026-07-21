package tsdns

import (
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"
)

// loadCache fetches all records from the repository and updates the in-memory cache.
func (s *Server) loadCache() error {
	timer := time.Now()
	records, err := s.repository.Find(s.ctx)
	if err != nil {
		s.metrics.RecordCacheRefresh("error")
		s.metrics.RecordRepositoryOp("find", "error", time.Since(timer))

		return err
	}
	s.metrics.RecordRepositoryOp("find", "success", time.Since(timer))

	newCache, wildcards, regexes := s.processRecords(records)

	// Sort wildcards by domain length descending to ensure the most specific match is found first.
	sort.SliceStable(wildcards, func(i, j int) bool {
		return len(wildcards[i].Domain) > len(wildcards[j].Domain)
	})

	s.mu.Lock()
	s.cache = newCache
	s.wildcardRecords = wildcards
	s.regexRecords = regexes
	s.mu.Unlock()

	s.metrics.RecordCacheRefresh("success")

	return nil
}

func (s *Server) processRecords(records []*Record) (map[string]*Record, []*Record, []*compiledRecord) {
	newCache := make(map[string]*Record)
	wildcards := make([]*Record, 0)
	regexes := make([]*compiledRecord, 0)

	for _, r := range records {
		domain := r.Domain
		newCache[domain] = r

		if s.tryProcessRegex(r, &regexes) {
			continue
		}

		if s.tryProcessAdvancedGlob(r, &regexes) {
			continue
		}

		if strings.HasPrefix(domain, "*") {
			wildcards = append(wildcards, r)
		}
	}

	return newCache, wildcards, regexes
}

func (s *Server) tryProcessRegex(r *Record, regexes *[]*compiledRecord) bool {
	if pattern, ok := strings.CutPrefix(r.Domain, "reg:"); ok {
		re, err := regexp.Compile(pattern)
		if err == nil {
			*regexes = append(*regexes, &compiledRecord{Record: r, re: re})
		} else {
			s.logger.Error("failed to compile regex", slog.String("domain", r.Domain), slog.Any("error", err))
		}

		return true
	}

	return false
}

func (s *Server) tryProcessAdvancedGlob(r *Record, regexes *[]*compiledRecord) bool {
	// Check for advanced glob/wildcard patterns (contains * or ? but not just a simple prefix *)
	// Or if it contains multiple *
	domain := r.Domain
	isAdvanced := strings.Contains(domain, "?") || strings.Count(domain, "*") > 1 ||
		(strings.Contains(domain, "*") && !strings.HasPrefix(domain, "*"))

	if isAdvanced {
		pattern := domainToRegex(domain)
		re, err := regexp.Compile(pattern)
		if err == nil {
			*regexes = append(*regexes, &compiledRecord{Record: r, re: re})
		} else {
			s.logger.Error("failed to compile glob as regex", slog.String("domain", domain), slog.Any("error", err))
		}

		return true
	}

	return false
}

func domainToRegex(domain string) string {
	// Escape dots and other regex special chars
	r := regexp.QuoteMeta(domain)
	// Convert escaped \* back to .* and \? to .
	r = strings.ReplaceAll(r, "\\*", ".*")
	r = strings.ReplaceAll(r, "\\?", ".")

	return "^" + r + "$"
}

const defaultCacheRefreshInterval = 30 * time.Second

// cacheUpdater periodically refreshes the in-memory cache from the repository.
func (s *Server) cacheUpdater() {
	interval := s.cacheRefreshInterval
	if interval <= 0 {
		interval = defaultCacheRefreshInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			err := s.loadCache()
			if err != nil {
				s.logger.Error("cache update error", slog.Any("error", err))
			}
		case <-s.ctx.Done():
			return
		}
	}
}
