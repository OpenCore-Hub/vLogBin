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
  isSessionSecretConfigured,
  authConfig,
} from "@/lib/auth/config";
import {
  getAuthRequest,
  getLoginSettings,
  getSession,
  validateSession,
} from "@/lib/auth/zitadel-session";
import {
  RememberedSession,
  getRememberedSessions,
} from "@/lib/auth/zitadel-sessions-store";
import { LockIcon, TerminalIcon } from "@/components/ui/icons";

export const metadata: Metadata = {
  title: "登录 · vLogBin",
};

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
  if (
    authConfig.mode === "oidc-custom-login" &&
    authRequestParam &&
    customLoginReady
  ) {
    try {
      [authRequest, loginSettings] = await Promise.all([
        getAuthRequest(authRequestParam),
        getLoginSettings(),
      ]);
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
            savedSessions.push(candidate);
          }
        } catch {
          // 过期/失效会话由继续会话 action 清理，登录页保持可操作。
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
              <CustomLoginForm
                authRequestId={authRequest.id}
                next={safeNext}
                loginSettings={loginSettings}
                savedSessions={savedSessions}
              />
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
