import "server-only";

export type AuthEvent =
  | "custom_login.identifier.success"
  | "custom_login.password.success"
  | "custom_login.mfa_required"
  | "custom_login.callback.completed"
  | "custom_login.hosted_fallback"
  | "custom_signup.created"
  | "custom_signup.email_verified"
  | "custom_signup.passkey_registered";

export function recordAuthEvent(
  event: AuthEvent,
  details: Record<string, string | number | boolean | undefined>,
): void {
  console.log(
    JSON.stringify({
      ts: new Date().toISOString(),
      event,
      ...details,
    }),
  );
}
