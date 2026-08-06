import "server-only";

import { Code, ConnectError } from "@connectrpc/connect";
import { create, timestampMs } from "@zitadel/client";
import { makeReqCtx } from "@zitadel/client/v2";
import { DurationSchema } from "@bufbuild/protobuf/wkt";
import { TextQueryMethod } from "@zitadel/proto/zitadel/object/v2/object_pb";
import { CredentialsCheckErrorSchema } from "@zitadel/proto/zitadel/message_pb";
import {
  CreateCallbackRequestSchema,
  GetAuthRequestRequestSchema,
  SessionSchema as OidcSessionSchema,
} from "@zitadel/proto/zitadel/oidc/v2/oidc_service_pb";
import {
  RequestChallenges,
  RequestChallengesSchema,
  RequestChallenges_WebAuthNSchema,
  UserVerificationRequirement,
} from "@zitadel/proto/zitadel/session/v2/challenge_pb";
import {
  Checks,
  CheckPasswordSchema,
  CheckOTPSchema,
  CheckTOTPSchema,
  CheckUserSchema,
  CheckWebAuthNSchema,
  CreateSessionRequestSchema,
  DeleteSessionRequestSchema,
  GetSessionRequestSchema,
  ListSessionsRequestSchema,
  SetSessionRequestSchema,
} from "@zitadel/proto/zitadel/session/v2/session_service_pb";
import {
  IDsQuerySchema,
  Session,
  SearchQuerySchema as SessionSearchQuerySchema,
  UserAgentSchema,
} from "@zitadel/proto/zitadel/session/v2/session_pb";
import { GetLoginSettingsRequestSchema } from "@zitadel/proto/zitadel/settings/v2/settings_service_pb";
import {
  EmailQuerySchema,
  LoginNameQuerySchema,
  OrganizationIdQuerySchema,
  PhoneQuerySchema,
  SearchQuerySchema as UserSearchQuerySchema,
  UserFieldName,
} from "@zitadel/proto/zitadel/user/v2/query_pb";
import {
  SendEmailVerificationCodeSchema,
  SetHumanEmailSchema,
} from "@zitadel/proto/zitadel/user/v2/email_pb";
import { PasswordSchema } from "@zitadel/proto/zitadel/user/v2/password_pb";
import { SetHumanProfileSchema } from "@zitadel/proto/zitadel/user/v2/user_pb";
import {
  AuthenticationMethodType,
  CreateUserRequestSchema,
  GetUserByIDRequestSchema,
  ListAuthenticationMethodTypesRequestSchema,
  ListUsersRequestSchema,
  ResendEmailCodeRequestSchema,
  VerifyEmailRequestSchema,
} from "@zitadel/proto/zitadel/user/v2/user_service_pb";
import {
  authConfig,
  isCustomLoginConfigured,
} from "./config";
import { getZitadelClients } from "./zitadel-client";
import {
  LoginSettingsSnapshot,
  SessionValidationResult,
  ZitadelSessionSnapshot,
  evaluateSessionFactors,
} from "./zitadel-mfa";

export type {
  LoginSettingsSnapshot,
  SessionValidationResult,
  ZitadelSessionSnapshot,
} from "./zitadel-mfa";
export { evaluateSessionFactors } from "./zitadel-mfa";

export type ZitadelErrorCode =
  | "not-configured"
  | "transport"
  | "invalid-response"
  | "not-found"
  | "permission-denied"
  | "conflict"
  | "rate-limited"
  | "mfa-required"
  | "session-invalid"
  | "unavailable"
  | "unknown";

export class ZitadelApiError extends Error {
  constructor(
    message: string,
    public readonly code: ZitadelErrorCode,
    public readonly id?: string,
    public readonly failedAttempts?: number,
    public readonly cause?: unknown,
  ) {
    super(message);
    this.name = "ZitadelApiError";
  }
}

export interface ZitadelUserHit {
  userId: string;
  username: string;
  loginNames: string[];
  preferredLoginName: string;
  displayName: string;
  email: string;
  emailVerified: boolean;
  organizationId: string;
  active: boolean;
}

export interface AuthRequestSnapshot {
  id: string;
  clientId: string;
  scope: string[];
  redirectUri: string;
  loginHint?: string;
  hintUserId?: string;
}

export interface CreateSessionInput {
  userId?: string;
  loginName?: string;
  password?: string;
  challenges?: RequestChallenges;
  fingerprintId?: string;
  ip?: string;
  description?: string;
  userAgentHeader?: Record<string, string[]>;
  lifetimeSeconds?: number;
}

export interface CreateSessionResult {
  sessionId: string;
  sessionToken: string;
  session: ZitadelSessionSnapshot;
}

export interface SetSessionInput {
  sessionId: string;
  sessionToken: string;
  password?: string;
  totpCode?: string;
  otpEmailCode?: string;
  otpSmsCode?: string;
  webAuthNAssertionData?: unknown;
  challenges?: RequestChallenges;
  lifetimeSeconds?: number;
}

export interface CreateUserInput {
  email: string;
  givenName: string;
  familyName: string;
  password?: string;
  organizationId?: string;
  sendEmailCode?: boolean;
  urlTemplate?: string;
}

function isCustomLoginActive(): asserts this is typeof authConfig & { mode: "oidc-custom-login" } {
  if (!isCustomLoginConfigured()) {
    throw new ZitadelApiError(
      "自建登录模式未配置：请设置 AUTH_MODE=oidc-custom-login 与 ZITADEL 服务凭据。",
      "not-configured",
    );
  }
}

function toApiError(err: unknown): ZitadelApiError {
  if (err instanceof ConnectError) {
    const credentials = err.findDetails(CredentialsCheckErrorSchema)[0];
    const code =
      err.code === Code.NotFound
        ? "not-found"
        : err.code === Code.PermissionDenied
          ? "permission-denied"
          : err.code === Code.AlreadyExists
            ? "conflict"
            : err.code === Code.ResourceExhausted
              ? "rate-limited"
              : err.code === Code.Unavailable
                ? "unavailable"
                : err.code === Code.InvalidArgument
                  ? "invalid-response"
                  : "unknown";
    return new ZitadelApiError(
      err.rawMessage || err.message,
      code,
      credentials?.id,
      credentials?.failedAttempts,
      err,
    );
  }
  if (err instanceof ZitadelApiError) {
    return err;
  }
  return new ZitadelApiError(
    err instanceof Error ? err.message : "ZITADEL API 调用失败",
    "transport",
    undefined,
    undefined,
    err,
  );
}

function verifiedAt(ts?: { verifiedAt?: unknown }): number {
  return ts?.verifiedAt && typeof ts.verifiedAt === "object" && "seconds" in ts.verifiedAt
    ? timestampMs(ts.verifiedAt as never)
    : 0;
}

function toSessionSnapshot(session: Session): ZitadelSessionSnapshot {
  const factors = session.factors;
  const user = factors?.user;
  return {
    id: session.id,
    expirationDate: session.expirationDate ? timestampMs(session.expirationDate) : null,
    user: user
      ? {
          id: user.id,
          loginName: user.loginName,
          displayName: user.displayName,
          organizationId: user.organizationId,
        }
      : null,
    passwordVerifiedAt: verifiedAt(factors?.password),
    webAuthN: factors?.webAuthN?.verifiedAt
      ? {
          verifiedAt: timestampMs(factors.webAuthN.verifiedAt),
          userVerified: factors.webAuthN.userVerified,
        }
      : null,
    intentVerifiedAt: verifiedAt(factors?.intent),
    totpVerifiedAt: verifiedAt(factors?.totp),
    otpSmsVerifiedAt: verifiedAt(factors?.otpSms),
    otpEmailVerifiedAt: verifiedAt(factors?.otpEmail),
  };
}

function toLoginSettingsSnapshot(settings: {
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
  secondFactors: unknown[];
  multiFactors: unknown[];
}): LoginSettingsSnapshot {
  return {
    allowUsernamePassword: settings.allowUsernamePassword,
    allowRegister: settings.allowRegister,
    allowExternalIdp: settings.allowExternalIdp,
    forceMfa: settings.forceMfa,
    forceMfaLocalOnly: settings.forceMfaLocalOnly,
    hidePasswordReset: settings.hidePasswordReset,
    ignoreUnknownUsernames: settings.ignoreUnknownUsernames,
    defaultRedirectUri: settings.defaultRedirectUri,
    disableLoginWithEmail: settings.disableLoginWithEmail,
    disableLoginWithPhone: settings.disableLoginWithPhone,
    secondFactors: settings.secondFactors.map(String),
    multiFactors: settings.multiFactors.map(String),
  };
}

function toUserHit(user: {
  userId: string;
  username: string;
  loginNames: string[];
  preferredLoginName: string;
  state: number;
  details?: { resourceOwner: string };
  type: {
    case: "human";
    value: {
      profile?: { displayName?: string };
      email?: { email: string; isVerified: boolean };
    };
  } | { case: "machine" } | { case: undefined };
}): ZitadelUserHit {
  return {
    userId: user.userId,
    username: user.username,
    loginNames: user.loginNames,
    preferredLoginName: user.preferredLoginName,
    displayName:
      user.type.case === "human"
        ? user.type.value.profile?.displayName || ""
        : "",
    organizationId: user.details?.resourceOwner || "",
    email:
      user.type.case === "human" ? user.type.value.email?.email || "" : "",
    emailVerified:
      user.type.case === "human"
        ? Boolean(user.type.value.email?.isVerified)
        : false,
    active: user.state === 1,
  };
}

function lifetimeMessage(seconds: number | undefined) {
  if (!seconds || seconds <= 0) {
    return undefined;
  }
  return create(DurationSchema, {
    seconds: BigInt(seconds),
    nanos: 0,
  });
}

function checksMessage(input: CreateSessionInput | SetSessionInput): Checks | undefined {
  const checks: Parameters<typeof create>[1] = {};
  if ("userId" in input && input.userId) {
    checks.user = create(CheckUserSchema, {
      search: { case: "userId", value: input.userId },
    });
  }
  if ("loginName" in input && input.loginName) {
    checks.user = create(CheckUserSchema, {
      search: { case: "loginName", value: input.loginName },
    });
  }
  if (input.password) {
    checks.password = create(CheckPasswordSchema, { password: input.password });
  }
  if ("totpCode" in input && input.totpCode) {
    checks.totp = create(CheckTOTPSchema, { code: input.totpCode });
  }
  if ("otpEmailCode" in input && input.otpEmailCode) {
    checks.otpEmail = create(CheckOTPSchema, { code: input.otpEmailCode });
  }
  if ("otpSmsCode" in input && input.otpSmsCode) {
    checks.otpSms = create(CheckOTPSchema, { code: input.otpSmsCode });
  }
  if ("webAuthNAssertionData" in input && input.webAuthNAssertionData) {
    checks.webAuthN = create(CheckWebAuthNSchema, {
      credentialAssertionData: input.webAuthNAssertionData as never,
    });
  }
  return Object.keys(checks).length > 0 ? (checks as Checks) : undefined;
}

function userAgentMessage(input: CreateSessionInput) {
  if (
    !input.fingerprintId &&
    !input.ip &&
    !input.description &&
    !input.userAgentHeader
  ) {
    return undefined;
  }
  return create(UserAgentSchema, {
    fingerprintId: input.fingerprintId,
    ip: input.ip,
    description: input.description,
    header: Object.fromEntries(
      Object.entries(input.userAgentHeader ?? {}).map(([key, values]) => [
        key,
        { values },
      ]),
    ),
  });
}

export async function searchUsers(input: {
  loginName?: string;
  email?: string;
  phone?: string;
  organizationId?: string;
}): Promise<ZitadelUserHit[]> {
  isCustomLoginActive();
  const queries: Parameters<typeof create>[1][] = [];
  if (input.loginName) {
    queries.push(
      create(UserSearchQuerySchema, {
        query: {
          case: "loginNameQuery",
          value: create(LoginNameQuerySchema, {
            loginName: input.loginName,
            method: TextQueryMethod.EQUALS_IGNORE_CASE,
          }),
        },
      }),
    );
  }
  if (input.email) {
    queries.push(
      create(UserSearchQuerySchema, {
        query: {
          case: "emailQuery",
          value: create(EmailQuerySchema, {
            emailAddress: input.email,
            method: TextQueryMethod.EQUALS_IGNORE_CASE,
          }),
        },
      }),
    );
  }
  if (input.phone) {
    queries.push(
      create(UserSearchQuerySchema, {
        query: {
          case: "phoneQuery",
          value: create(PhoneQuerySchema, {
            number: input.phone,
            method: TextQueryMethod.EQUALS_IGNORE_CASE,
          }),
        },
      }),
    );
  }
  if (input.organizationId) {
    queries.push(
      create(UserSearchQuerySchema, {
        query: {
          case: "organizationIdQuery",
          value: create(OrganizationIdQuerySchema, {
            organizationId: input.organizationId,
          }),
        },
      }),
    );
  }
  if (queries.length === 0) {
    throw new ZitadelApiError("缺少用户搜索条件。", "invalid-response");
  }

  try {
    const clients = await getZitadelClients();
    const response = await clients.user.listUsers(
      create(ListUsersRequestSchema, {
        query: { offset: BigInt(0), limit: 10, asc: true },
        sortingColumn: UserFieldName.USER_NAME,
        queries: queries as never,
      }),
    );
    return response.result.map(toUserHit);
  } catch (err) {
    throw toApiError(err);
  }
}

export async function createSession(
  input: CreateSessionInput,
): Promise<CreateSessionResult> {
  isCustomLoginActive();
  try {
    const clients = await getZitadelClients();
    const response = await clients.session.createSession(
      create(CreateSessionRequestSchema, {
        checks: checksMessage(input),
        metadata: {},
        challenges: input.challenges,
        userAgent: userAgentMessage(input),
        lifetime: lifetimeMessage(input.lifetimeSeconds),
      }),
    );
    if (!response.sessionId || !response.sessionToken) {
      throw new ZitadelApiError(
        "ZITADEL 未返回 sessionId / sessionToken。",
        "invalid-response",
      );
    }
    const session = await getSession({
      sessionId: response.sessionId,
      sessionToken: response.sessionToken,
    });
    return {
      sessionId: response.sessionId,
      sessionToken: response.sessionToken,
      session,
    };
  } catch (err) {
    throw toApiError(err);
  }
}

export async function setSession(
  input: SetSessionInput,
): Promise<CreateSessionResult> {
  isCustomLoginActive();
  try {
    const clients = await getZitadelClients();
    const response = await clients.session.setSession(
      create(SetSessionRequestSchema, {
        sessionId: input.sessionId,
        sessionToken: input.sessionToken,
        checks: checksMessage(input),
        metadata: {},
        challenges: input.challenges,
        lifetime: lifetimeMessage(input.lifetimeSeconds),
      }),
    );
    if (!response.sessionToken) {
      throw new ZitadelApiError("ZITADEL 未返回新 sessionToken。", "invalid-response");
    }
    const session = await getSession({
      sessionId: input.sessionId,
      sessionToken: response.sessionToken,
    });
    return {
      sessionId: input.sessionId,
      sessionToken: response.sessionToken,
      session,
    };
  } catch (err) {
    throw toApiError(err);
  }
}

export async function requestOtpChallenge(input: {
  sessionId: string;
  sessionToken: string;
  method: "email" | "sms";
}): Promise<{ sessionToken: string }> {
  const challenges = create(RequestChallengesSchema, {
    otpEmail:
      input.method === "email"
        ? {
            deliveryType: {
              case: "sendCode",
              value: {},
            },
          }
        : undefined,
    otpSms:
      input.method === "sms"
        ? {
            returnCode: false,
          }
        : undefined,
  });
  const result = await setSession({
    sessionId: input.sessionId,
    sessionToken: input.sessionToken,
    challenges,
  });
  return { sessionToken: result.sessionToken };
}

export async function requestWebAuthnChallenge(input: {
  sessionId: string;
  sessionToken: string;
  method: "passkey" | "u2f";
  domain: string;
}): Promise<{ sessionToken: string; options: unknown }> {
  isCustomLoginActive();
  try {
    const clients = await getZitadelClients();
    const response = await clients.session.setSession(
      create(SetSessionRequestSchema, {
        sessionId: input.sessionId,
        sessionToken: input.sessionToken,
        metadata: {},
        challenges: create(RequestChallengesSchema, {
          webAuthN: create(RequestChallenges_WebAuthNSchema, {
            domain: input.domain,
            userVerificationRequirement:
              input.method === "passkey"
                ? UserVerificationRequirement.REQUIRED
                : UserVerificationRequirement.DISCOURAGED,
          }),
        }),
      }),
    );
    if (
      !response.sessionToken ||
      !response.challenges?.webAuthN?.publicKeyCredentialRequestOptions
    ) {
      throw new ZitadelApiError(
        "ZITADEL 未返回 WebAuthN challenge。",
        "invalid-response",
      );
    }
    return {
      sessionToken: response.sessionToken,
      options: response.challenges.webAuthN.publicKeyCredentialRequestOptions,
    };
  } catch (err) {
    throw toApiError(err);
  }
}

export async function submitWebAuthnAssertion(input: {
  sessionId: string;
  sessionToken: string;
  assertionData: unknown;
}): Promise<CreateSessionResult> {
  return setSession({
    sessionId: input.sessionId,
    sessionToken: input.sessionToken,
    webAuthNAssertionData: input.assertionData,
  });
}

export async function getSession(input: {
  sessionId: string;
  sessionToken: string;
}): Promise<ZitadelSessionSnapshot> {
  isCustomLoginActive();
  try {
    const clients = await getZitadelClients();
    const response = await clients.session.getSession(
      create(GetSessionRequestSchema, input),
    );
    if (!response.session) {
      throw new ZitadelApiError("ZITADEL 未返回 session。", "not-found");
    }
    return toSessionSnapshot(response.session);
  } catch (err) {
    throw toApiError(err);
  }
}

export async function listSessions(ids: string[]): Promise<ZitadelSessionSnapshot[]> {
  isCustomLoginActive();
  if (ids.length === 0) {
    return [];
  }
  try {
    const clients = await getZitadelClients();
    const response = await clients.session.listSessions(
      create(ListSessionsRequestSchema, {
        queries: [
          create(SessionSearchQuerySchema, {
            query: {
              case: "idsQuery",
              value: create(IDsQuerySchema, { ids }),
            },
          }),
        ],
      }),
    );
    return response.sessions.map(toSessionSnapshot);
  } catch (err) {
    throw toApiError(err);
  }
}

export async function deleteSession(input: {
  sessionId: string;
  sessionToken: string;
}): Promise<void> {
  isCustomLoginActive();
  try {
    const clients = await getZitadelClients();
    await clients.session.deleteSession(create(DeleteSessionRequestSchema, input));
  } catch (err) {
    throw toApiError(err);
  }
}

const loginSettingsCache = new Map<
  string,
  { expiresAt: number; promise: Promise<LoginSettingsSnapshot> }
>();

export async function getLoginSettings(
  organizationId?: string,
): Promise<LoginSettingsSnapshot> {
  isCustomLoginActive();
  const key = organizationId || "instance";
  const cached = loginSettingsCache.get(key);
  if (cached && cached.expiresAt > Date.now()) {
    return cached.promise;
  }

  const promise = (async () => {
    try {
      const clients = await getZitadelClients();
      const response = await clients.settings.getLoginSettings(
        create(GetLoginSettingsRequestSchema, {
          ctx: makeReqCtx(organizationId),
        }),
      );
      if (!response.settings) {
        throw new ZitadelApiError("ZITADEL 未返回登录设置。", "invalid-response");
      }
      return toLoginSettingsSnapshot(response.settings);
    } catch (err) {
      throw toApiError(err);
    }
  })();

  loginSettingsCache.set(key, { expiresAt: Date.now() + 5 * 60 * 1000, promise });
  return promise;
}

export async function listAuthenticationMethodTypes(
  userId: string,
): Promise<AuthenticationMethodType[]> {
  isCustomLoginActive();
  try {
    const clients = await getZitadelClients();
    const response = await clients.user.listAuthenticationMethodTypes(
      create(ListAuthenticationMethodTypesRequestSchema, {
        userId,
      }),
    );
    return response.authMethodTypes;
  } catch (err) {
    throw toApiError(err);
  }
}

export async function getUserByID(userId: string): Promise<ZitadelUserHit> {
  isCustomLoginActive();
  try {
    const clients = await getZitadelClients();
    const response = await clients.user.getUserByID(
      create(GetUserByIDRequestSchema, { userId }),
    );
    if (!response.user) {
      throw new ZitadelApiError("用户不存在。", "not-found");
    }
    return toUserHit(response.user);
  } catch (err) {
    throw toApiError(err);
  }
}

export async function validateSession(
  session: ZitadelSessionSnapshot,
): Promise<SessionValidationResult> {
  if (!session.user) {
    return { valid: false, reason: "no-primary" };
  }

  const settings = await getLoginSettings(session.user.organizationId || undefined);
  const authMethods = await listAuthenticationMethodTypes(session.user.id);
  const factorResult = evaluateSessionFactors(session, settings, authMethods);
  if (!factorResult.valid) {
    return factorResult;
  }

  if (process.env.EMAIL_VERIFICATION === "true") {
    const user = await getUserByID(session.user.id);
    if (!user.emailVerified) {
      return { valid: false, reason: "email-not-verified" };
    }
  }

  return { valid: true, reason: "ok" };
}

export async function isSessionValid(
  session: ZitadelSessionSnapshot,
): Promise<boolean> {
  const result = await validateSession(session);
  return result.valid;
}

export async function getAuthRequest(
  authRequestId: string,
): Promise<AuthRequestSnapshot> {
  isCustomLoginActive();
  try {
    const clients = await getZitadelClients();
    const response = await clients.oidc.getAuthRequest(
      create(GetAuthRequestRequestSchema, { authRequestId }),
    );
    if (!response.authRequest) {
      throw new ZitadelApiError("授权请求不存在。", "not-found");
    }
    return {
      id: response.authRequest.id,
      clientId: response.authRequest.clientId,
      scope: response.authRequest.scope,
      redirectUri: response.authRequest.redirectUri,
      loginHint: response.authRequest.loginHint,
      hintUserId: response.authRequest.hintUserId,
    };
  } catch (err) {
    throw toApiError(err);
  }
}

export function isSafeCallbackUrl(url: string): boolean {
  try {
    const parsed = new URL(url);
    const expected = new URL(
      authConfig.zitadel.redirectUri || `${authConfig.baseUrl}/auth/callback`,
    );
    return (
      parsed.origin === expected.origin &&
      parsed.pathname === expected.pathname
    );
  } catch {
    return false;
  }
}

export async function createCallback(input: {
  authRequestId: string;
  sessionId: string;
  sessionToken: string;
}): Promise<{ callbackUrl: string }> {
  isCustomLoginActive();
  try {
    const clients = await getZitadelClients();
    const response = await clients.oidc.createCallback(
      create(CreateCallbackRequestSchema, {
        authRequestId: input.authRequestId,
        callbackKind: {
          case: "session",
          value: create(OidcSessionSchema, {
            sessionId: input.sessionId,
            sessionToken: input.sessionToken,
          }),
        },
      }),
    );
    if (!response.callbackUrl || !isSafeCallbackUrl(response.callbackUrl)) {
      throw new ZitadelApiError(
        "ZITADEL 返回的回调地址不在可信范围。",
        "invalid-response",
      );
    }
    return { callbackUrl: response.callbackUrl };
  } catch (err) {
    throw toApiError(err);
  }
}

export async function createUser(
  input: CreateUserInput,
): Promise<{ userId: string; emailCode?: string }> {
  isCustomLoginActive();
  try {
    const clients = await getZitadelClients();
    const response = await clients.user.createUser(
      create(CreateUserRequestSchema, {
        organizationId: input.organizationId || "",
        userType: {
          case: "human",
          value: {
            profile: create(SetHumanProfileSchema, {
              givenName: input.givenName,
              familyName: input.familyName,
              displayName: `${input.givenName} ${input.familyName}`.trim(),
            }),
            email: create(SetHumanEmailSchema, {
              email: input.email,
              verification: input.sendEmailCode
                ? {
                    case: "sendCode",
                    value: input.urlTemplate ? { urlTemplate: input.urlTemplate } : {},
                  }
                : { case: "isVerified", value: false },
            }),
            passwordType: input.password
              ? {
                  case: "password",
                  value: create(PasswordSchema, {
                    password: input.password,
                    changeRequired: false,
                  }),
                }
              : { case: undefined },
          },
        },
      }),
    );
    if (!response.id) {
      throw new ZitadelApiError("ZITADEL 未返回 userId。", "invalid-response");
    }
    return { userId: response.id, emailCode: response.emailCode };
  } catch (err) {
    throw toApiError(err);
  }
}

export async function verifyEmail(input: {
  userId: string;
  verificationCode: string;
}): Promise<void> {
  isCustomLoginActive();
  try {
    const clients = await getZitadelClients();
    await clients.user.verifyEmail(
      create(VerifyEmailRequestSchema, input),
    );
  } catch (err) {
    throw toApiError(err);
  }
}

export async function resendEmailCode(userId: string): Promise<void> {
  isCustomLoginActive();
  try {
    const clients = await getZitadelClients();
    await clients.user.resendEmailCode(
      create(ResendEmailCodeRequestSchema, {
        userId,
        verification: {
          case: "sendCode",
          value: create(SendEmailVerificationCodeSchema, {}),
        },
      }),
    );
  } catch (err) {
    throw toApiError(err);
  }
}
