import { describe, expect, it } from "vitest";
import {
  classifyPolicyWriteFailure,
  clearPendingPolicyWrite,
  findPendingPolicyWrite,
  freezePolicyBody,
  getPendingPolicyWrite,
  policyBodyFingerprint,
  setPendingPolicyWrite,
} from "./withdrawal-policy-state";

describe("withdrawal policy UI state helpers", () => {
  it("deep-freezes a confirmation body and fingerprints object key order consistently", () => {
    const source = {
      version: 4,
      config: { turnover_multiple: "0.25" },
      reason: "调整",
    };
    const frozen = freezePolicyBody(source);
    expect(Object.isFrozen(frozen)).toBe(true);
    expect(Object.isFrozen(frozen.config)).toBe(true);
    expect(policyBodyFingerprint(frozen)).toBe(
      policyBodyFingerprint({
        reason: "调整",
        config: { turnover_multiple: "0.25" },
        version: 4,
      }),
    );
    expect(source.config.turnover_multiple).toBe("0.25");
  });

  it("keeps unknown requests isolated in page-session state and classifies outcomes", () => {
    const key = "withdrawal-policy-test-isolated";
    const intent = {
      brandId: "brand-a",
      key: "same-key",
      body: { version: 2 },
    };
    setPendingPolicyWrite(key, intent);
    expect(getPendingPolicyWrite(key)).toEqual(intent);
    expect(
      findPendingPolicyWrite<typeof intent>(
        (value) => value.key === "same-key",
      ),
    ).toEqual(intent);
    clearPendingPolicyWrite(key);
    expect(getPendingPolicyWrite(key)).toBeNull();
    expect(classifyPolicyWriteFailure(0)).toBe("uncertain");
    expect(classifyPolicyWriteFailure(503)).toBe("uncertain");
    expect(classifyPolicyWriteFailure(409)).toBe("conflict");
    expect(classifyPolicyWriteFailure(400)).toBe("definitive");
  });
});
