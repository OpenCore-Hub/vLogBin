import { describe, expect, it } from "vitest";
import { AuthenticationMethodType } from "@zitadel/proto/zitadel/user/v2/user_service_pb";
import {
  LoginSettingsSnapshot,
  ZitadelSessionSnapshot,
  evaluateSessionFactors,
} from "./zitadel-mfa";

const baseSettings: LoginSettingsSnapshot = {
  allowUsernamePassword: true,
  allowRegister: true,
  allowExternalIdp: true,
  forceMfa: false,
  forceMfaLocalOnly: false,
  hidePasswordReset: false,
  ignoreUnknownUsernames: false,
  defaultRedirectUri: "/console",
  disableLoginWithEmail: false,
  disableLoginWithPhone: false,
  secondFactors: [],
  multiFactors: [],
};

function session(overrides: Partial<ZitadelSessionSnapshot> = {}): ZitadelSessionSnapshot {
  return {
    id: "session-1",
    expirationDate: null,
    user: {
      id: "user-1",
      loginName: "alice@example.com",
      displayName: "Alice",
      organizationId: "org-1",
    },
    passwordVerifiedAt: 1_700_000_000_000,
    webAuthN: null,
    intentVerifiedAt: 0,
    totpVerifiedAt: 0,
    otpSmsVerifiedAt: 0,
    otpEmailVerifiedAt: 0,
    ...overrides,
  };
}

describe("evaluateSessionFactors", () => {
  it("accepts password-only when no MFA is configured", () => {
    expect(
      evaluateSessionFactors(session(), baseSettings, [
        AuthenticationMethodType.PASSWORD,
      ]),
    ).toEqual({ valid: true, reason: "ok" });
  });

  it("rejects password-only when TOTP is configured but not verified", () => {
    expect(
      evaluateSessionFactors(
        session(),
        baseSettings,
        [AuthenticationMethodType.PASSWORD, AuthenticationMethodType.TOTP],
      ),
    ).toEqual({ valid: false, reason: "mfa-required" });
  });

  it("accepts password plus verified TOTP", () => {
    expect(
      evaluateSessionFactors(
        session({ totpVerifiedAt: 1_700_000_000_000 }),
        baseSettings,
        [AuthenticationMethodType.PASSWORD, AuthenticationMethodType.TOTP],
      ),
    ).toEqual({ valid: true, reason: "ok" });
  });

  it("does not treat U2F as a primary factor", () => {
    expect(
      evaluateSessionFactors(
        session({
          passwordVerifiedAt: 0,
          webAuthN: { verifiedAt: 1_700_000_000_000, userVerified: false },
        }),
        baseSettings,
        [AuthenticationMethodType.U2F],
      ),
    ).toEqual({ valid: false, reason: "no-primary" });
  });

  it("accepts password plus verified U2F", () => {
    expect(
      evaluateSessionFactors(
        session({
          webAuthN: { verifiedAt: 1_700_000_000_000, userVerified: false },
        }),
        baseSettings,
        [AuthenticationMethodType.PASSWORD, AuthenticationMethodType.U2F],
      ),
    ).toEqual({ valid: true, reason: "ok" });
  });

  it("accepts passwordless WebAuthN even when MFA is forced", () => {
    expect(
      evaluateSessionFactors(
        session({
          passwordVerifiedAt: 0,
          webAuthN: { verifiedAt: 1_700_000_000_000, userVerified: true },
        }),
        { ...baseSettings, forceMfa: true },
        [AuthenticationMethodType.PASSKEY],
      ),
    ).toEqual({ valid: true, reason: "ok" });
  });

  it("accepts an IDP intent as primary factor when MFA is local-only", () => {
    expect(
      evaluateSessionFactors(
        session({
          passwordVerifiedAt: 0,
          intentVerifiedAt: 1_700_000_000_000,
        }),
        { ...baseSettings, forceMfaLocalOnly: true },
        [AuthenticationMethodType.PASSWORD],
      ),
    ).toEqual({ valid: true, reason: "ok" });
  });

  it("still requires an additional factor for IDP login when MFA is forced globally", () => {
    expect(
      evaluateSessionFactors(
        session({
          passwordVerifiedAt: 0,
          intentVerifiedAt: 1_700_000_000_000,
        }),
        { ...baseSettings, forceMfa: true },
        [AuthenticationMethodType.PASSWORD],
      ),
    ).toEqual({ valid: false, reason: "mfa-required" });
  });

  it("rejects an IDP primary factor when configured MFA is not verified", () => {
    expect(
      evaluateSessionFactors(
        session({
          passwordVerifiedAt: 0,
          intentVerifiedAt: 1_700_000_000_000,
        }),
        baseSettings,
        [
          AuthenticationMethodType.PASSWORD,
          AuthenticationMethodType.TOTP,
        ],
      ),
    ).toEqual({ valid: false, reason: "mfa-required" });
  });

  it("requires an additional factor when forceMfa is enabled", () => {
    expect(
      evaluateSessionFactors(
        session(),
        { ...baseSettings, forceMfa: true },
        [AuthenticationMethodType.PASSWORD],
      ),
    ).toEqual({ valid: false, reason: "mfa-required" });
  });

  it("rejects an expired session", () => {
    expect(
      evaluateSessionFactors(
        session({ expirationDate: Date.now() - 1 }),
        baseSettings,
        [AuthenticationMethodType.PASSWORD],
      ),
    ).toEqual({ valid: false, reason: "expired" });
  });
});
