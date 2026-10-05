package drawfeed

import (
	"errors"
	"net"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ErrLimit reports a source configuration that exceeds a documented bound.
var ErrLimit = errors.New("draw feed limit exceeded")

const (
	maxConfigNameBytes     = 120
	maxConfigEndpointBytes = 2048
	maxConfigSelectorBytes = 500
	maxConfigCredentialRef = 120
	maxConfigPriority      = 10000
)

var (
	configUUIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	credentialPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_.:/-]{0,119}$`)
	dnsLabelPattern   = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)
)

// SourceConfig describes one bounded, externally configured draw source.
// CredentialRef names a secret stored elsewhere; it must never contain a
// credential value.
type SourceConfig struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	Priority      int    `json:"priority"`
	Enabled       bool   `json:"enabled"`
	Endpoint      string `json:"endpoint"`
	Selector      string `json:"selector"`
	CredentialRef string `json:"credential_ref"`
}

// ValidateSources checks source configuration without mutating it or doing
// network, DNS, or other external lookups. Priorities and IDs are unique even
// for disabled sources.
func ValidateSources(configs []SourceConfig) error {
	if len(configs) > maxSourceCount {
		return ErrLimit
	}

	seenIDs := make(map[string]struct{}, len(configs))
	seenPriorities := make(map[int]struct{}, len(configs))
	for _, config := range configs {
		if !configUUIDPattern.MatchString(config.ID) {
			return ErrInvalid
		}
		idKey := strings.ToLower(config.ID)
		if _, exists := seenIDs[idKey]; exists {
			return ErrInvalid
		}
		seenIDs[idKey] = struct{}{}

		if !utf8.ValidString(config.Name) || strings.TrimSpace(config.Name) == "" || strings.IndexByte(config.Name, 0) >= 0 {
			return ErrInvalid
		}
		if len(config.Name) > maxConfigNameBytes {
			return ErrLimit
		}
		if config.Type != "api" && config.Type != "dom" {
			return ErrInvalid
		}
		if config.Priority < 1 {
			return ErrInvalid
		}
		if config.Priority > maxConfigPriority {
			return ErrLimit
		}
		if _, exists := seenPriorities[config.Priority]; exists {
			return ErrInvalid
		}
		seenPriorities[config.Priority] = struct{}{}

		if err := validateEndpoint(config.Endpoint); err != nil {
			return err
		}
		if !utf8.ValidString(config.Selector) || strings.IndexByte(config.Selector, 0) >= 0 {
			return ErrInvalid
		}
		if config.Type == "api" {
			if config.Selector != "" {
				return ErrInvalid
			}
		} else {
			if strings.TrimSpace(config.Selector) == "" {
				return ErrInvalid
			}
			if len(config.Selector) > maxConfigSelectorBytes {
				return ErrLimit
			}
		}

		if !utf8.ValidString(config.CredentialRef) {
			return ErrInvalid
		}
		if len(config.CredentialRef) > maxConfigCredentialRef {
			return ErrLimit
		}
		if config.CredentialRef != "" && !credentialPattern.MatchString(config.CredentialRef) {
			return ErrInvalid
		}
	}
	return nil
}

// FeedSources projects configs to resolver inputs in the same order, retaining
// stable persisted IDs and priorities. It always returns a non-nil slice.
func FeedSources(configs []SourceConfig) []Source {
	sources := make([]Source, len(configs))
	for i, config := range configs {
		sources[i] = Source{
			ID:       config.ID,
			Type:     config.Type,
			Priority: config.Priority,
			Enabled:  config.Enabled,
		}
	}
	return sources
}

func validateEndpoint(endpoint string) error {
	if !utf8.ValidString(endpoint) || endpoint == "" {
		return ErrInvalid
	}
	if len(endpoint) > maxConfigEndpointBytes {
		return ErrLimit
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Opaque != "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || strings.Contains(endpoint, "#") {
		return ErrInvalid
	}
	if parsed.Host == "" || parsed.Hostname() == "" {
		return ErrInvalid
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return ErrInvalid
	}
	if strings.HasSuffix(parsed.Host, ":") {
		return ErrInvalid
	}
	host := parsed.Hostname()
	if net.ParseIP(host) != nil || !validPublicDNSName(host) {
		return ErrInvalid
	}
	return nil
}

func validPublicDNSName(host string) bool {
	if len(host) == 0 || len(host) > 253 || !strings.Contains(host, ".") {
		return false
	}
	labels := strings.Split(host, ".")
	// Reject inet_aton/browser-style shorthand and encoded numeric hosts too.
	// Real adapters still need DNS/redirect/private-address checks at request time.
	if !strings.ContainsAny(labels[len(labels)-1], "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		return false
	}
	for _, label := range labels {
		if !dnsLabelPattern.MatchString(label) {
			return false
		}
	}
	lower := strings.ToLower(host)
	for _, suffix := range []string{".localhost", ".local", ".internal", ".test", ".onion", ".invalid", ".example"} {
		if strings.HasSuffix(lower, suffix) {
			return false
		}
	}
	return true
}
