import { describe, expect, it } from "vitest";
import { isWithdrawalQualification, type WithdrawalQualification } from "./withdrawal-qualification";

const valid: WithdrawalQualification = {
  brand_id: "11111111-1111-4111-8111-111111111111",
  member_id: "22222222-2222-4222-8222-222222222222",
  account_id: "33333333-3333-4333-8333-333333333333",
  base_points: "9007199254740993",
  valid_points: "9007199254740994",
  valid_order_count: "2",
  credit_numerator: "18014398509481987",
  credit_denominator: "2",
  meets_turnover: true,
  cycle_from_at: "2026-10-07T11:00:00+08:00",
  cycle_from_version: "7",
  cutoff_at: "2026-10-07T12:00:00Z",
  cutoff_version: "8",
};

describe("withdrawal qualification snapshot validation", () => {
  it("validates large exact fractional credit and preserves decimal strings", () => {
    expect(isWithdrawalQualification(valid)).toBe(true);
  });

  it("rejects invalid denominators, inconsistent claims, malformed dates, and private additions", () => {
    expect(isWithdrawalQualification({ ...valid, credit_denominator: "0" })).toBe(false);
    expect(isWithdrawalQualification({ ...valid, credit_numerator: "18014398509481985", meets_turnover: true })).toBe(false);
    expect(isWithdrawalQualification({ ...valid, credit_numerator: "18014398509481987", meets_turnover: false })).toBe(false);
    expect(isWithdrawalQualification({ ...valid, account_id: undefined })).toBe(false);
    expect(isWithdrawalQualification({ ...valid, internal_reason: "private" })).toBe(false);
    expect(isWithdrawalQualification({ ...valid, cutoff_at: "2026-10-07" })).toBe(false);
  });

  it("enforces the cycle cursor, int64 fields, canonical decimals, and maximum numeric length", () => {
    expect(isWithdrawalQualification({ ...valid, cycle_from_at: null, cycle_from_version: "1" })).toBe(false);
    expect(isWithdrawalQualification({ ...valid, cycle_from_at: "2026-10-07T12:01:00Z" })).toBe(false);
    expect(isWithdrawalQualification({ ...valid, cycle_from_at: "2026-10-07T12:00:00.000000002Z", cutoff_at: "2026-10-07T12:00:00.000000001Z" })).toBe(false);
    expect(isWithdrawalQualification({ ...valid, cycle_from_version: "9" })).toBe(false);
    expect(isWithdrawalQualification({ ...valid, cycle_from_version: "8" })).toBe(true);
    expect(isWithdrawalQualification({ ...valid, cycle_from_at: "2026-10-07T11:00:00+23:59" })).toBe(true);
    expect(isWithdrawalQualification({ ...valid, cycle_from_at: "2026-10-07T11:00:00+24:00" })).toBe(false);
    expect(isWithdrawalQualification({ ...valid, cycle_from_at: "2026-10-07T11:00:00+01:60" })).toBe(false);
    expect(isWithdrawalQualification({ ...valid, cutoff_version: "9223372036854775808" })).toBe(false);
    expect(isWithdrawalQualification({ ...valid, valid_points: "01" })).toBe(false);
    expect(isWithdrawalQualification({ ...valid, credit_numerator: "1".repeat(16385) })).toBe(false);
    expect(isWithdrawalQualification({ ...valid, cycle_from_at: null, cycle_from_version: "0" })).toBe(true);
  });
});
