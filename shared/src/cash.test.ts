import { describe, it, expect } from "vitest";
import { exchangeResult, minor, decimal, isCashConfig } from "./cash";
import { cashCatalog } from "./cash-catalog";
describe("currency money rules", () => {
  it("floors destination amounts without float drift or roundtrip gain", () => {
    expect(exchangeResult("1", "7", false, "USDT")).toBe("0.142857");
    expect(exchangeResult("1", "7", false, "CNY")).toBe("0.14");
    expect(exchangeResult("123.456789", "7", true, "USDT")).toBe("864.19");
    expect(exchangeResult("0.142857", "7", true, "USDT")).toBe("0.99");
    expect(decimal(minor("0.1") + minor("0.2"))).toBe("0.3");
  });
  it("requires the complete currency set and translations", () => {
    expect(isCashConfig({ version: 0, rules: [] })).toBe(false);
    expect(Object.keys(cashCatalog.en).sort()).toEqual(
      Object.keys(cashCatalog["zh-CN"]).sort(),
    );
  });
});
