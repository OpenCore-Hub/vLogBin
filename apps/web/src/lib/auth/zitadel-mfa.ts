import { AuthenticationMethodType } from "@zitadel/proto/zitadel/user/v2/user_service_pb";

export interface ZitadelSessionSnapshot {
  id: string;
  expirationDate: number | null;
  user: {
    id: string;
    loginName: string;
    displayName: string;
    organizationId: string;
  } | null;
  passwordVerifiedAt: number;
  webAuthN: { verifiedAt: number; userVerified: boolean } | null;
  intentVerifiedAt: number;
  totpVerifiedAt: number;
  otpSmsVerifiedAt: number;
  otpEmailVerifiedAt: number;
}

export interface LoginSettingsSnapshot {
  allowUsernamePassword: boolean;
  allowRegister: boolean;
  allowExternalIdp: boolean;
  forceMfa: boolean;
  forceMfaLocalOnly: boolean;
  hidePasswordReset: boolean;
  ignoreUnknownUsernames: boolean;
  defaultRedirectUri: string;
  disableLoginWithEmail: boolean;
  disableLoginWithPhone: boolean;
  secondFactors: string[];
  multiFactors: string[];
}

export type SessionValidationReason =
  | "ok"
  | "expired"
  | "no-primary"
  | "mfa-required"
  | "email-not-verified";

export type SessionValidationResult = {
  valid: boolean;
  reason: SessionValidationReason;
};

function hasPrimaryFactor(session: ZitadelSessionSnapshot): boolean {
  return Boolean(
    session.passwordVerifiedAt ||
      (session.webAuthN?.verifiedAt && session.webAuthN.userVerified) ||
      session.intentVerifiedAt,
  );
}

function shouldEnforceMfa(
  session: ZitadelSessionSnapshot,
  settings: LoginSettingsSnapshot | undefined,
): boolean {
  if (!settings) {
    return false;
  }
  if (session.webAuthN?.verifiedAt && session.webAuthN.userVerified) {
    return false;
  }
  if (settings.forceMfa) {
    return true;
  }
  if (settings.forceMfaLocalOnly) {
    if (session.intentVerifiedAt) {
      return false;
    }
    if (session.passwordVerifiedAt) {
      return true;
    }
  }
  return false;
}

export function evaluateSessionFactors(
  session: ZitadelSessionSnapshot,
  settings: LoginSettingsSnapshot | undefined,
  authMethods: AuthenticationMethodType[],
): SessionValidationResult {
  if (session.expirationDate !== null && session.expirationDate <= Date.now()) {
    return { valid: false, reason: "expired" };
  }
  if (!session.user || !hasPrimaryFactor(session)) {
    return { valid: false, reason: "no-primary" };
  }

  const mfaMethods = authMethods.filter((method) =>
    [
      AuthenticationMethodType.TOTP,
      AuthenticationMethodType.OTP_EMAIL,
      AuthenticationMethodType.OTP_SMS,
      AuthenticationMethodType.U2F,
    ].includes(method),
  );

  let mfaValid = true;
  if (mfaMethods.length > 0) {
    const totpValid =
      mfaMethods.includes(AuthenticationMethodType.TOTP) &&
      session.totpVerifiedAt > 0;
    const otpEmailValid =
      mfaMethods.includes(AuthenticationMethodType.OTP_EMAIL) &&
      session.otpEmailVerifiedAt > 0;
    const otpSmsValid =
      mfaMethods.includes(AuthenticationMethodType.OTP_SMS) &&
      session.otpSmsVerifiedAt > 0;
    const u2fValid =
      mfaMethods.includes(AuthenticationMethodType.U2F) &&
      Boolean(session.webAuthN?.verifiedAt);
    mfaValid = totpValid || otpEmailValid || otpSmsValid || u2fValid;
  } else if (shouldEnforceMfa(session, settings)) {
    mfaValid =
      session.totpVerifiedAt > 0 ||
      session.otpEmailVerifiedAt > 0 ||
      session.otpSmsVerifiedAt > 0 ||
      Boolean(session.webAuthN?.verifiedAt);
  }

  if (!mfaValid) {
    return { valid: false, reason: "mfa-required" };
  }

  return { valid: true, reason: "ok" };
}
