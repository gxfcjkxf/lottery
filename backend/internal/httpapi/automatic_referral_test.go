package httpapi

import (
	"context"
	"testing"
)

func TestAutomaticReferralKeepsAgentCodesIndependent(t *testing.T) {
	f := pointsFixture(t)
	agentGrantRoot(t, f.managementHTTP)
	grantJoinCodeRoot(t, f.managementHTTP)
	enabled := f.call("PUT", "/api/v1/admin/agent-policy", "automatic-referral-policy-001", f.token, managedBrand, agentPolicyBody(true, 3, "0.1"))
	if enabled.Code != 200 {
		t.Fatal(enabled.Code, enabled.Body.String())
	}
	created := f.call("POST", "/api/v1/admin/agents", "automatic-referral-agent-001", f.token, managedBrand, agentNodeBody(2, f.memberID, nil, nil, "0.08"))
	if created.Code != 201 {
		t.Fatal(created.Code, created.Body.String())
	}
	var node agentNode
	managedData(t, created, &node)
	if node.MemberID == f.memberID {
		t.Fatal("agent reused the member identity")
	}
	wrongOwner := joinCodeCreateBody(f.memberID)
	wrongOwner["kind"], wrongOwner["agent_id"] = "agent", node.ID
	denied := f.call("POST", "/api/v1/admin/join-codes", "automatic-referral-wrong-owner-001", f.token, managedBrand, wrongOwner)
	if denied.Code != 404 {
		t.Fatal("member cannot own an independent agent code", denied.Code, denied.Body.String())
	}
	ctx := context.Background()
	var before, after string
	if err := f.pool.QueryRow(ctx, `SELECT to_jsonb(c)::text FROM join_codes c WHERE brand_id=$1 AND owner_member_id=$2 AND agent_id=$3 AND kind='agent'`, managedBrand, node.MemberID, node.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := f.pool.Exec(ctx, `SELECT ensure_member_referral($1,$2)`, managedBrand, f.memberID); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.pool.QueryRow(ctx, `SELECT to_jsonb(c)::text FROM join_codes c WHERE brand_id=$1 AND owner_member_id=$2 AND agent_id=$3 AND kind='agent'`, managedBrand, node.MemberID, node.ID).Scan(&after); err != nil || before != after {
		t.Fatal("agent code changed", err)
	}
	var referrals, agents, ledger int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE kind='referral' AND owner_member_id=$2),count(*) FILTER(WHERE kind='agent' AND owner_member_id=$3),(SELECT count(*) FROM point_ledger_entries) FROM join_codes WHERE brand_id=$1`, managedBrand, f.memberID, node.MemberID).Scan(&referrals, &agents, &ledger); err != nil || referrals != 1 || agents != 1 || ledger != 0 {
		t.Fatal(referrals, agents, ledger, err)
	}
}
