package main

import (
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
)

const proposalTestBrand = "11111111-1111-4111-8111-111111111111"
const proposalTestCycle = "22222222-2222-4222-8222-222222222222"
const proposalTestID = "33333333-3333-4333-8333-333333333333"

func TestParseCommissionProposalArgsStrictGrammar(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		mode  string
		valid bool
	}{
		{"source", []string{"commission-recovery-source", proposalTestBrand, proposalTestCycle}, "source", true},
		{"propose", []string{"commission-recovery-propose", proposalTestBrand, proposalTestCycle, commissionProposalConfirmation}, "propose", true},
		{"review", []string{"commission-recovery-review", proposalTestBrand, proposalTestID, "2", commissionProposalConfirmation}, "review", true},
		{"get", []string{"commission-recovery-get", proposalTestBrand, proposalTestID}, "get", true},
		{"unknown command", []string{"commission-recovery-other"}, "", false},
		{"extra source argument", []string{"commission-recovery-source", proposalTestBrand, proposalTestCycle, "extra"}, "", false},
		{"uppercase uuid", []string{"commission-recovery-get", proposalTestBrand, strings.ToUpper("abcdefab-cdef-4abc-8def-abcdefabcdef")}, "", false},
		{"nonhex uuid", []string{"commission-recovery-get", "1111111g-1111-4111-8111-111111111111", proposalTestID}, "", false},
		{"missing confirmation", []string{"commission-recovery-propose", proposalTestBrand, proposalTestCycle}, "", false},
		{"wrong confirmation", []string{"commission-recovery-review", proposalTestBrand, proposalTestID, "1", "--confirm=anything-else"}, "", false},
		{"zero version", []string{"commission-recovery-review", proposalTestBrand, proposalTestID, "0", commissionProposalConfirmation}, "", false},
		{"leading zero version", []string{"commission-recovery-review", proposalTestBrand, proposalTestID, "01", commissionProposalConfirmation}, "", false},
		{"signed version", []string{"commission-recovery-review", proposalTestBrand, proposalTestID, "+1", commissionProposalConfirmation}, "", false},
		{"negative version", []string{"commission-recovery-review", proposalTestBrand, proposalTestID, "-1", commissionProposalConfirmation}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, recognized, err := parseCommissionProposalArgs(tt.args)
			if tt.name == "unknown command" {
				if recognized || err != nil {
					t.Fatalf("expected unknown command passthrough, got recognized=%v err=%v", recognized, err)
				}
				return
			}
			if !tt.valid {
				if !recognized || err == nil {
					t.Fatalf("expected recognized invalid command, got recognized=%v err=%v", recognized, err)
				}
				return
			}
			if !recognized || err != nil || command.mode != tt.mode {
				t.Fatalf("unexpected parse: command=%+v recognized=%v err=%v", command, recognized, err)
			}
		})
	}
}

func TestCommissionProposalEnvironmentValidation(t *testing.T) {
	base := commissionProposalEnvironment{bearer: strings.Repeat("A", 43)}
	for _, mode := range []string{"source", "get"} {
		if err := validateCommissionProposalEnvironment(commissionProposalCommand{mode: mode}, base); err != nil {
			t.Fatalf("%s should need only bearer: %v", mode, err)
		}
	}
	writeEnv := commissionProposalEnvironment{
		bearer: base.bearer,
		reason: "record-only evidence review",
		digest: strings.Repeat("a", 64),
		key:    "recovery-key-0001",
	}
	for _, mode := range []string{"propose", "review"} {
		if err := validateCommissionProposalEnvironment(commissionProposalCommand{mode: mode}, writeEnv); err != nil {
			t.Fatalf("%s environment rejected: %v", mode, err)
		}
	}
	for _, env := range []commissionProposalEnvironment{
		{},
		{bearer: strings.Repeat("A", 42)},
		{bearer: strings.Repeat("+", 43)},
		{bearer: base.bearer, reason: " ", digest: writeEnv.digest, key: writeEnv.key},
		{bearer: base.bearer, reason: strings.Repeat("x", 501), digest: writeEnv.digest, key: writeEnv.key},
		{bearer: base.bearer, reason: writeEnv.reason, digest: strings.Repeat("A", 64), key: writeEnv.key},
		{bearer: base.bearer, reason: writeEnv.reason, digest: writeEnv.digest, key: "short"},
	} {
		if err := validateCommissionProposalEnvironment(commissionProposalCommand{mode: "review"}, env); err == nil {
			t.Fatalf("invalid environment accepted: %+v", env)
		}
	}
}

func TestCommissionProposalMissingBearerPrecedesConfigLoad(t *testing.T) {
	t.Setenv("LOTTERY_COMMISSION_RECOVERY_BEARER", "")
	t.Setenv("LOTTERY_COMMISSION_RECOVERY_REASON", "")
	t.Setenv("LOTTERY_COMMISSION_RECOVERY_DIGEST", "")
	t.Setenv("LOTTERY_COMMISSION_RECOVERY_KEY", "")
	t.Setenv("DATABASE_URL", "")
	previousArgs := os.Args
	os.Args = []string{"platform", "commission-recovery-get", proposalTestBrand, proposalTestID}
	t.Cleanup(func() { os.Args = previousArgs })
	err := run(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || err.Error() != "commission recovery bearer configuration unavailable" {
		t.Fatalf("expected bearer preflight error before config loading, got %v", err)
	}
	t.Setenv("LOTTERY_COMMISSION_RECOVERY_BEARER", strings.Repeat("A", 43))
	os.Args = []string{"platform", "commission-recovery-propose", proposalTestBrand, proposalTestCycle, commissionProposalConfirmation}
	err = run(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || err.Error() != "commission recovery reason configuration unavailable" {
		t.Fatalf("expected write environment preflight error before config loading, got %v", err)
	}
}
