package drawfeed

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func validSourceConfig() SourceConfig {
	return SourceConfig{
		ID:       "123e4567-e89b-12d3-a456-426614174000",
		Name:     "Primary draw feed",
		Type:     "api",
		Priority: 10,
		Enabled:  true,
		Endpoint: "https://feeds.example.com/draws",
	}
}

func TestValidateSourcesAcceptsValidConfigurations(t *testing.T) {
	tests := []struct {
		name    string
		configs []SourceConfig
	}{
		{name: "empty list", configs: []SourceConfig{}},
		{name: "api", configs: []SourceConfig{validSourceConfig()}},
		{name: "dom", configs: []SourceConfig{func() SourceConfig {
			config := validSourceConfig()
			config.Type = "dom"
			config.Selector = "#result .numbers"
			config.CredentialRef = "vault:draw/source1"
			return config
		}()}},
		{name: "sixteen sources", configs: sourceConfigs(maxSourceCount)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateSources(test.configs); err != nil {
				t.Fatalf("ValidateSources() error = %v", err)
			}
		})
	}
}

func TestValidateSourcesRejectsInvalidConfigurations(t *testing.T) {
	tests := []struct {
		name string
		edit func([]SourceConfig) []SourceConfig
		want error
	}{
		{name: "malformed uuid", edit: editFirst(func(c *SourceConfig) { c.ID = "123e4567-e89b-12d3-a456-42661417400z" }), want: ErrInvalid},
		{name: "uuid with whitespace", edit: editFirst(func(c *SourceConfig) { c.ID = " 123e4567-e89b-12d3-a456-426614174000" }), want: ErrInvalid},
		{name: "duplicate uuid across case", edit: func(configs []SourceConfig) []SourceConfig {
			configs = append(configs, configs[0])
			configs[1].ID = strings.ToUpper(configs[1].ID)
			configs[1].Priority++
			return configs
		}, want: ErrInvalid},
		{name: "empty name", edit: editFirst(func(c *SourceConfig) { c.Name = "" }), want: ErrInvalid},
		{name: "blank name", edit: editFirst(func(c *SourceConfig) { c.Name = " \t" }), want: ErrInvalid},
		{name: "oversized name", edit: editFirst(func(c *SourceConfig) { c.Name = strings.Repeat("n", maxConfigNameBytes+1) }), want: ErrLimit},
		{name: "invalid source type", edit: editFirst(func(c *SourceConfig) { c.Type = "manual" }), want: ErrInvalid},
		{name: "zero priority", edit: editFirst(func(c *SourceConfig) { c.Priority = 0 }), want: ErrInvalid},
		{name: "priority over limit", edit: editFirst(func(c *SourceConfig) { c.Priority = maxConfigPriority + 1 }), want: ErrLimit},
		{name: "duplicate priority across disabled source", edit: func(configs []SourceConfig) []SourceConfig {
			second := configs[0]
			second.ID = "223e4567-e89b-12d3-a456-426614174000"
			second.Enabled = false
			return append(configs, second)
		}, want: ErrInvalid},
		{name: "empty endpoint", edit: editFirst(func(c *SourceConfig) { c.Endpoint = "" }), want: ErrInvalid},
		{name: "oversized endpoint", edit: editFirst(func(c *SourceConfig) {
			c.Endpoint = "https://feeds.example.com/" + strings.Repeat("a", maxConfigEndpointBytes)
		}), want: ErrLimit},
		{name: "http endpoint", edit: editFirst(func(c *SourceConfig) { c.Endpoint = "http://feeds.example.com/draws" }), want: ErrInvalid},
		{name: "endpoint userinfo", edit: editFirst(func(c *SourceConfig) { c.Endpoint = "https://user:pass@feeds.example.com/draws" }), want: ErrInvalid},
		{name: "endpoint query", edit: editFirst(func(c *SourceConfig) { c.Endpoint = "https://feeds.example.com/draws?token=secret" }), want: ErrInvalid},
		{name: "empty endpoint query", edit: editFirst(func(c *SourceConfig) { c.Endpoint = "https://feeds.example.com/draws?" }), want: ErrInvalid},
		{name: "endpoint fragment", edit: editFirst(func(c *SourceConfig) { c.Endpoint = "https://feeds.example.com/draws#section" }), want: ErrInvalid},
		{name: "empty endpoint fragment", edit: editFirst(func(c *SourceConfig) { c.Endpoint = "https://feeds.example.com/draws#" }), want: ErrInvalid},
		{name: "non-https port", edit: editFirst(func(c *SourceConfig) { c.Endpoint = "https://feeds.example.com:8443/draws" }), want: ErrInvalid},
		{name: "invalid port", edit: editFirst(func(c *SourceConfig) { c.Endpoint = "https://feeds.example.com:bad/draws" }), want: ErrInvalid},
		{name: "private ipv4 literal", edit: editFirst(func(c *SourceConfig) { c.Endpoint = "https://192.168.1.10/draws" }), want: ErrInvalid},
		{name: "ipv4 shorthand", edit: editFirst(func(c *SourceConfig) { c.Endpoint = "https://127.1/draws" }), want: ErrInvalid},
		{name: "encoded numeric host", edit: editFirst(func(c *SourceConfig) { c.Endpoint = "https://0x7f.1/draws" }), want: ErrInvalid},
		{name: "public ipv4 literal", edit: editFirst(func(c *SourceConfig) { c.Endpoint = "https://8.8.8.8/draws" }), want: ErrInvalid},
		{name: "ipv6 literal", edit: editFirst(func(c *SourceConfig) { c.Endpoint = "https://[2001:4860:4860::8888]/draws" }), want: ErrInvalid},
		{name: "localhost hostname", edit: editFirst(func(c *SourceConfig) { c.Endpoint = "https://localhost/draws" }), want: ErrInvalid},
		{name: "internal hostname", edit: editFirst(func(c *SourceConfig) { c.Endpoint = "https://draw.internal/draws" }), want: ErrInvalid},
		{name: "test hostname", edit: editFirst(func(c *SourceConfig) { c.Endpoint = "https://feed.test/draws" }), want: ErrInvalid},
		{name: "onion hostname", edit: editFirst(func(c *SourceConfig) { c.Endpoint = "https://feed.onion/draws" }), want: ErrInvalid},
		{name: "fixture invalid hostname", edit: editFirst(func(c *SourceConfig) { c.Endpoint = "https://fixtureexample.invalid/draws" }), want: ErrInvalid},
		{name: "malformed dns label", edit: editFirst(func(c *SourceConfig) { c.Endpoint = "https://-bad.example.com/draws" }), want: ErrInvalid},
		{name: "api selector not empty", edit: editFirst(func(c *SourceConfig) { c.Selector = "#result" }), want: ErrInvalid},
		{name: "dom selector empty", edit: editFirst(func(c *SourceConfig) { c.Type = "dom" }), want: ErrInvalid},
		{name: "dom selector too long", edit: editFirst(func(c *SourceConfig) { c.Type = "dom"; c.Selector = strings.Repeat("x", maxConfigSelectorBytes+1) }), want: ErrLimit},
		{name: "selector nul", edit: editFirst(func(c *SourceConfig) { c.Type = "dom"; c.Selector = "#x\x00y" }), want: ErrInvalid},
		{name: "credential reference secret punctuation", edit: editFirst(func(c *SourceConfig) { c.CredentialRef = "token=secret" }), want: ErrInvalid},
		{name: "credential reference too long", edit: editFirst(func(c *SourceConfig) { c.CredentialRef = "a" + strings.Repeat("x", maxConfigCredentialRef) }), want: ErrLimit},
		{name: "too many sources", edit: func([]SourceConfig) []SourceConfig { return sourceConfigs(maxSourceCount + 1) }, want: ErrLimit},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configs := test.edit([]SourceConfig{validSourceConfig()})
			if err := ValidateSources(configs); !errors.Is(err, test.want) {
				t.Fatalf("ValidateSources() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestValidateSourcesValidatesDisabledSources(t *testing.T) {
	config := validSourceConfig()
	config.Enabled = false
	config.Endpoint = "http://fixtureexample.invalid"
	if err := ValidateSources([]SourceConfig{config}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ValidateSources() error = %v, want %v", err, ErrInvalid)
	}
}

func TestValidateSourcesDoesNotMutateInput(t *testing.T) {
	configs := sourceConfigs(2)
	before := append([]SourceConfig(nil), configs...)
	if err := ValidateSources(configs); err != nil {
		t.Fatalf("ValidateSources() error = %v", err)
	}
	for i := range configs {
		if configs[i] != before[i] {
			t.Fatalf("ValidateSources() mutated config %d", i)
		}
	}
}

func TestFeedSourcesProjectsWithoutSorting(t *testing.T) {
	configs := sourceConfigs(2)
	configs[0].Priority = 20
	configs[1].Priority = 3
	configs[1].Enabled = false
	sources := FeedSources(configs)
	if len(sources) != 2 {
		t.Fatalf("FeedSources() returned %d sources, want 2", len(sources))
	}
	for i, config := range configs {
		if sources[i] != (Source{ID: config.ID, Type: config.Type, Priority: config.Priority, Enabled: config.Enabled}) {
			t.Fatalf("FeedSources()[%d] = %#v, does not preserve projection of %#v", i, sources[i], config)
		}
	}
}

func TestFeedSourcesReturnsNonNilEmptySlice(t *testing.T) {
	sources := FeedSources(nil)
	if sources == nil || len(sources) != 0 {
		t.Fatalf("FeedSources(nil) = %#v, want non-nil empty slice", sources)
	}
}

func sourceConfigs(count int) []SourceConfig {
	configs := make([]SourceConfig, count)
	for i := range configs {
		configs[i] = validSourceConfig()
		configs[i].ID = fmt.Sprintf("%08x-e89b-12d3-a456-426614174000", i+1)
		configs[i].Priority = i + 1
	}
	return configs
}

func editFirst(edit func(*SourceConfig)) func([]SourceConfig) []SourceConfig {
	return func(configs []SourceConfig) []SourceConfig {
		edit(&configs[0])
		return configs
	}
}
