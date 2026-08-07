import type { Metadata } from "next";
import Link from "next/link";
import { startOidcLogin } from "./login-actions";
import { OperatorTokenForm } from "./token-form";
import { CustomLoginForm } from "./custom-login-form";
import { Logo } from "@/components/brand/logo";
import { Button } from "@/components/ui/button";
import { Alert } from "@/components/ui/feedback";
import {
  isOidcConfigured,
  isCustomLoginConfigured,
  isCustomLoginAllowedForUser,
  isSessionSecretConfigured,
  authConfig,
} from "@/lib/auth/config";
import {
  getActiveIdentityProviders,
  getAuthRequest,
  getLoginSettings,
  getSession,
  listAuthenticationMethodTypes,
  resolveOrganizationFromScopes,
  validateSession,
} from "@/lib/auth/zitadel-session";
import {
  RememberedSession,
  getRememberedSessions,
} from "@/lib/auth/zitadel-sessions-store";
import { getLoginFlow } from "@/lib/auth/login-flow";
import { AuthenticationMethodType } from "@zitadel/proto/zitadel/user/v2/user_service_pb";
import { CustomLoginActionState } from "./login-state";
import { LockIcon, TerminalIcon } from "@/components/ui/icons";

export const metadata: Metadata = {
  title: "登录 · vLogBin",
};

function mfaMethodLabel(method: AuthenticationMethodType): string {
  switch (method) {
    case AuthenticationMethodType.TOTP:
      return "TOTP";
    case AuthenticationMethodType.OTP_EMAIL:
      return "OTP_EMAIL";
    case AuthenticationMethodType.OTP_SMS:
      return "OTP_SMS";
    case AuthenticationMethodType.U2F:
      return "U2F";
    case AuthenticationMethodType.PASSKEY:
      return "PASSKEY";
    default:
      return "UNKNOWN";
  }
}

export default async function LoginPage({
  searchParams,
}: {
  searchParams: Promise<{ next?: string; authRequest?: string }>;
}) {
  const { next, authRequest: authRequestParam } = await searchParams;
  const safeNext =
    next && next.startsWith("/") && !next.startsWith("//")
      ? next
      : "/console";
  const oidcReady = isOidcConfigured();
  const sessionReady = isSessionSecretConfigured();
  const customLoginReady = isCustomLoginConfigured();
  let authRequest: Awaited<ReturnType<typeof getAuthRequest>> | null = null;
  let loginSettings: Awaited<ReturnType<typeof getLoginSettings>> | null = null;
  const savedSessions: RememberedSession[] = [];
  let customLoginError: string | null = null;
  let identityProviders: Awaited<
    ReturnType<typeof getActiveIdentityProviders>
  > = [];
  let pendingLoginState: CustomLoginActionState | undefined;
  if (
    authConfig.mode === "oidc-custom-login" &&
    authRequestParam &&
    customLoginReady
  ) {
    try {
      authRequest = await getAuthRequest(authRequestParam);
      const organizationId = await resolveOrganizationFromScopes(
        authRequest.scope,
      );
      loginSettings = await getLoginSettings(organizationId);
      if (!isCustomLoginAllowedForUser(authRequest.hintUserId)) {
        customLoginError = "该账号不在自建登录灰度范围，请使用托管登录。";
      }
      if (loginSettings?.allowExternalIdp) {
        try {
          identityProviders = await getActiveIdentityProviders(organizationId);
        } catch {
          // 身份源列表失败时仍保留本地账号登录入口。
        }
      }
      const remembered = await getRememberedSessions();
      for (const candidate of remembered) {
        if (
          authRequest.hintUserId &&
          candidate.userId !== authRequest.hintUserId
        ) {
          continue;
        }
        try {
          const session = await getSession({
            sessionId: candidate.sessionId,
            sessionToken: candidate.sessionToken,
          });
          const validation = await validateSession(session);
          if (validation.valid) {
            if (
              organizationId &&
              session.user?.organizationId !== organizationId
            ) {
              continue;
            }
            savedSessions.push(candidate);
          }
        } catch {
          // 过期/失效会话由继续会话 action 清理，登录页保持可操作。
        }
      }
      const pendingFlow = await getLoginFlow();
      if (
        pendingFlow?.sessionId &&
        pendingFlow.userId &&
        pendingFlow.authRequestId === authRequestParam
      ) {
        try {
          const pendingSession = await getSession({
            sessionId: pendingFlow.sessionId,
            sessionToken: pendingFlow.sessionToken,
          });
          const pendingValidation = await validateSession(pendingSession);
          if (
            !pendingValidation.valid &&
            pendingValidation.reason === "mfa-required" &&
            pendingSession.user
          ) {
            const [pendingSettings, rawMethods] = await Promise.all([
              getLoginSettings(
                pendingSession.user.organizationId || undefined,
              ),
              listAuthenticationMethodTypes(pendingSession.user.id),
            ]);
            const methods = rawMethods
              .map(mfaMethodLabel)
              .filter((method) =>
                ["TOTP", "OTP_EMAIL", "OTP_SMS", "U2F", "PASSKEY"].includes(
                  method,
                ),
              );
            pendingLoginState = {
              ok: false,
              step: "mfa",
              loginName: pendingFlow.loginName,
              userId: pendingSession.user.id,
              sessionId: pendingFlow.sessionId,
              mfaMethods: methods,
              mfaSetupRequired:
                methods.length === 0 &&
                Boolean(pendingSettings.mfaInitSkipLifetimeSeconds),
            };
          }
        } catch {
          // 待完成 MFA 的会话若已失效，让用户重新登录即可。
        }
      }
    } catch {
      customLoginError = "登录请求已失效，请重新开始。";
    }
  }

  return (
    <div className="flex min-h-dvh flex-col">
      <header className="flex items-center justify-between border-b border-border px-6 py-4">
        <Link href="/" aria-label="vLogBin 首页">
          <Logo />
        </Link>
        <Link
          href="/"
          className="text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          返回首页
        </Link>
      </header>

      <main className="flex flex-1 items-center justify-center px-4 py-12">
        <div className="w-full max-w-sm">
          <div className="mb-8 flex flex-col items-center gap-3 text-center">
            <span className="flex size-14 items-center justify-center rounded-2xl bg-brand-700 text-white shadow-[var(--shadow-premium)]">
              <LockIcon size={22} />
            </span>
            <div>
              <h1 className="text-xl font-semibold text-foreground">
                登录 vLogBin
              </h1>
              <p className="mt-1 text-sm text-muted-foreground">
                进入你的控制台，管理计费与用量
              </p>
            </div>
          </div>

          {!sessionReady && (
            <Alert variant="danger" title="服务未就绪" className="mb-4">
              未配置 <code className="font-mono">SESSION_SECRET</code>
              （至少 32 字符），无法建立安全会话。
            </Alert>
          )}

          {authConfig.mode === "oidc-custom-login" &&
          authRequestParam &&
          customLoginReady ? (
          authRequest && loginSettings ? (
            isCustomLoginAllowedForUser(authRequest.hintUserId) ? (
              <CustomLoginForm
                authRequestId={authRequest.id}
                next={safeNext}
                loginSettings={loginSettings}
                savedSessions={savedSessions}
                identityProviders={identityProviders}
                initialState={pendingLoginState}
              />
            ) : (
              <div className="space-y-4">
                <Alert variant="warning" title="托管登录">
                  {customLoginError}
                </Alert>
                <form action={startOidcLogin}>
                  <input type="hidden" name="next" value={safeNext} />
                  <Button type="submit" size="lg" className="w-full">
                    使用 ZITADEL 登录
                  </Button>
                </form>
              </div>
            )
          ) : (
              <Alert variant="danger" title="登录不可用">
                {customLoginError || "自建登录配置不完整。"}
              </Alert>
            )
          ) : authConfig.mode === "oidc" ||
            authConfig.mode === "oidc-custom-login" ? (
            oidcReady ? (
              <form action={startOidcLogin}>
                <input type="hidden" name="next" value={safeNext} />
                <Button type="submit" size="lg" className="w-full">
                  <TerminalIcon size={15} />
                  {authConfig.mode === "oidc-custom-login"
                    ? "开始登录"
                    : "使用 ZITADEL 登录"}
                </Button>
              </form>
            ) : (
              <Alert
                variant="warning"
                title="OIDC 未配置"
                className="mb-4"
              >
                请设置 <code className="font-mono">ZITADEL_URL</code> 与{" "}
                <code className="font-mono">ZITADEL_CLIENT_ID</code>；本地开发可设置{" "}
                <code className="font-mono">AUTH_MODE=operator-token</code>{" "}
                使用令牌登录。
              </Alert>
            )
          ) : (
            <OperatorTokenForm next={safeNext} />
          )}

          <p className="mt-6 text-center text-sm text-muted-foreground">
            没有账号？{" "}
            <Link
              href={`/signup?next=${encodeURIComponent(safeNext)}`}
              className="font-medium text-brand-600 transition-colors hover:text-brand-700"
            >
              立即注册
            </Link>
          </p>

          <p className="mt-3 text-center text-xs text-muted-foreground">
            登录即代表你同意 vLogBin 的服务条款与隐私政策
          </p>
        </div>
      </main>
    </div>
  );
}
