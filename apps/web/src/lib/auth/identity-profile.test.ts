import { describe, expect, it } from "vitest";
import { splitDisplayName } from "./identity-profile";

describe("splitDisplayName", () => {
  it("splits a full display name into given and family names", () => {
    expect(splitDisplayName("Kilgore Trout")).toEqual({
      givenName: "Kilgore",
      familyName: "Trout",
    });
  });

  it("keeps multi-word family names together", () => {
    expect(splitDisplayName("Ada Lovelace Byron")).toEqual({
      givenName: "Ada",
      familyName: "Lovelace Byron",
    });
  });

  it("returns only a given name for a single token", () => {
    expect(splitDisplayName("Prince")).toEqual({ givenName: "Prince" });
  });

  it("returns no names when the display name is missing or blank", () => {
    expect(splitDisplayName(undefined)).toEqual({});
    expect(splitDisplayName("   ")).toEqual({});
  });
});
