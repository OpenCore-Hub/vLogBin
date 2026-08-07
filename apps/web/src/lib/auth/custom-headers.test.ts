import { describe, expect, it, vi } from "vitest";
import {
  applyCustomHeaders,
  parseCustomHeaders,
} from "./custom-headers";

describe("parseCustomHeaders", () => {
  it("parses comma separated key:value pairs", () => {
    expect(
      parseCustomHeaders("Host:http://zitadel-internal:8080, X-Tenant: acme"),
    ).toEqual([
      { key: "Host", value: "http://zitadel-internal:8080" },
      { key: "X-Tenant", value: "acme" },
    ]);
  });

  it("represents removal headers without a value", () => {
    expect(parseCustomHeaders("X-Remove-Me:")).toEqual([
      { key: "X-Remove-Me", value: undefined },
    ]);
  });

  it("skips malformed entries", () => {
    expect(parseCustomHeaders("NoColonHere, X-Tenant:acme")).toEqual([
      { key: "X-Tenant", value: "acme" },
    ]);
  });

  it("applies set and delete actions", () => {
    const target = {
      set: vi.fn(),
      delete: vi.fn(),
    };
    applyCustomHeaders(target, "X-Set:value, X-Delete:");

    expect(target.set).toHaveBeenCalledWith("X-Set", "value");
    expect(target.delete).toHaveBeenCalledWith("X-Delete");
  });
});
