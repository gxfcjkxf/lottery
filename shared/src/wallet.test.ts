import { describe, expect, it } from "vitest";
import { isWalletDTO, normalizeLedgerSnapshots, normalizeSourceBuckets, WALLET_SOURCES, WALLET_STATES, type WalletDTO } from "./wallet";

const ids = {
  account_id: "11111111-1111-4111-8111-111111111111",
  brand_id: "22222222-2222-4222-8222-222222222222",
  member_id: "33333333-3333-4333-8333-333333333333",
};
const buckets = (sources: readonly string[]) => Object.fromEntries(sources.map((source) => [source, Object.fromEntries(WALLET_STATES.map((state) => [state, "0"]))]));
const current: WalletDTO = {
  ...ids, version: 1, display_points: "0", available_points: "0", frozen_points: "0", withdrawal_points: "0",
  recharge_points: "0", winning_points: "0", gift_points: "0", commission_points: "0", manual_frozen_points: "0", system_frozen_points: "0",
  by_source: buckets(WALLET_SOURCES) as WalletDTO["by_source"],
};

describe("four-source wallet contract", () => {
  it("requires a closed current wallet DTO with commission points and buckets", () => {
    expect(isWalletDTO(current)).toBe(true);
    const { commission_points: _commissionPoints, ...withoutTotal } = current;
    expect(isWalletDTO(withoutTotal)).toBe(false);
    const { commission: _commission, ...withoutSource } = current.by_source;
    expect(isWalletDTO({ ...current, by_source: withoutSource })).toBe(false);
    expect(isWalletDTO({ ...current, by_source: buckets(["recharge", "winning", "gift"]) })).toBe(false);
    expect(isWalletDTO({ ...current, extra: "not closed" })).toBe(false);
  });

  it("normalizes immutable three-source snapshots in memory and keeps four-source snapshots strict", () => {
    const old = buckets(["recharge", "winning", "gift"]);
    const normalized = normalizeSourceBuckets(old);
    expect(Object.keys(normalized ?? {})).toEqual(WALLET_SOURCES);
    expect(normalized?.commission).toEqual({ available: "0", manual_frozen: "0", system_frozen: "0", withdrawal: "0" });
    expect(Object.keys(old)).toEqual(["recharge", "winning", "gift"]);
    expect(normalizeSourceBuckets({ ...buckets(WALLET_SOURCES), unknown: {} })).toBeNull();
    expect(normalizeSourceBuckets({ ...buckets(WALLET_SOURCES), commission: { available: "0", manual_frozen: "0", system_frozen: "0" } })).toBeNull();
    expect(normalizeSourceBuckets({ ...buckets(WALLET_SOURCES), commission: { available: "-0", manual_frozen: "0", system_frozen: "0", withdrawal: "0" } }, true)).toBeNull();
    expect(normalizeSourceBuckets({ recharge: { available: "5000000000000000000", manual_frozen: "0", system_frozen: "0", withdrawal: "0" }, winning: { available: "5000000000000000000", manual_frozen: "0", system_frozen: "0", withdrawal: "0" }, gift: { available: "0", manual_frozen: "0", system_frozen: "0", withdrawal: "0" }, commission: { available: "0", manual_frozen: "0", system_frozen: "0", withdrawal: "0" } })).toBeNull();
  });

  it("checks wallet aggregates against each asymmetric source bucket", () => {
    const by_source = buckets(WALLET_SOURCES) as WalletDTO["by_source"];
    by_source.recharge.available = "2";
    by_source.winning.available = "3";
    by_source.gift.available = "5";
    by_source.commission.available = "7";
    by_source.commission.manual_frozen = "11";
    const wallet: WalletDTO = {
      ...current, display_points: "28", available_points: "17", frozen_points: "11", withdrawal_points: "0",
      recharge_points: "2", winning_points: "3", gift_points: "5", commission_points: "7",
      manual_frozen_points: "11", system_frozen_points: "0", by_source,
    };
    expect(isWalletDTO(wallet)).toBe(true);
    expect(isWalletDTO({ ...wallet, commission_points: "6" })).toBe(false);
    expect(isWalletDTO({ ...wallet, available_points: "10" })).toBe(false);
  });

  it("normalizes cached historical point-entry receipts without changing the frozen receipt", () => {
    const legacy = buckets(["recharge", "winning", "gift"]);
    const receipt = { id: "frozen", before_snapshot: legacy, delta_snapshot: legacy, after_snapshot: legacy };
    const normalized = normalizeLedgerSnapshots(receipt);
    expect(normalized?.after_snapshot.commission.available).toBe("0");
    expect(Object.keys(legacy)).toEqual(["recharge", "winning", "gift"]);
  });
});
