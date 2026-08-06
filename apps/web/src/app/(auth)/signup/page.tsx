import type { Metadata } from "next";
import Link from "next/link";
import { startOidcSignup } from "./signup-actions";
import { CustomSignupForm } from "./custom-signup-form";
import { Logo } from "@/components/brand/logo";
import { Button, LinkButton } from "@/components/ui/button";
import { Alert, InfoNote } from "@/components/ui/feedback";
import {
  isOidcConfigured,
  isCustomLoginConfigured,
  isSessionSecretConfigured,
  authConfig,
} from "@/lib/auth/config";
import {
  getAuthRequest,
  getLoginSettings,
} from "@/lib/auth/zitadel-session";
import { UsersIcon, TerminalIcon } from "@/components/ui/icons";

export const metadata: Metadata = {
  title: "注册 · vLogBin",
};

export default async function SignupPage({
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
    } catch {
      customLoginError = "注册请求已失效，请重新开始。";
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
            <span className="flex size-12 items-center justify-center rounded-xl bg-brand-50 text-brand-600">
              <UsersIcon size={22} />
            </span>
            <div>
              <h1 className="text-xl font-semibold text-foreground">
                创建 vLogBin 工作空间
              </h1>
              <p className="mt-1 text-sm text-muted-foreground">
                注册即获得独立的 test / live 双环境
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
              <CustomSignupForm
                authRequestId={authRequest.id}
                next={safeNext}
                loginSettings={loginSettings}
              />
            ) : (
              <Alert variant="danger" title="注册不可用">
                {customLoginError || "自建登录配置不完整。"}
              </Alert>
            )
          ) : authConfig.mode === "oidc" ||
            authConfig.mode === "oidc-custom-login" ? (
            oidcReady ? (
              <>
                <form action={startOidcSignup}>
                  <input type="hidden" name="next" value={safeNext} />
                  <Button type="submit" size="lg" className="w-full">
                    <TerminalIcon size={15} />
                {authConfig.mode === "oidc-custom-login"
                  ? "开始注册"
                  : "使用 ZITADEL 创建账号"}
                  </Button>
                </form>
                <p className="mt-4 text-center text-xs text-muted-foreground">
                  首个注册用户将自动成为工作空间管理员，
                  <br />
                  登录后可继续创建 Provider 接入计费。
                </p>
              </>
            ) : (
              <Alert variant="warning" title="OIDC 未配置" className="mb-4">
                请设置 <code className="font-mono">ZITADEL_URL</code> 与{" "}
                <code className="font-mono">ZITADEL_CLIENT_ID</code>；本地开发可设置{" "}
                <code className="font-mono">AUTH_MODE=operator-token</code>{" "}
                使用令牌登录。
              </Alert>
            )
          ) : (
            <div className="flex flex-col gap-3">
              <InfoNote>
                令牌登录模式不支持自助注册，请联系管理员获取访问令牌。
              </InfoNote>
              <LinkButton href={`/login?next=${encodeURIComponent(safeNext)}`} className="w-full">
                前往登录
              </LinkButton>
            </div>
          )}

          <p className="mt-6 text-center text-sm text-muted-foreground">
            已有账号？{" "}
            <Link
              href={`/login?next=${encodeURIComponent(safeNext)}`}
              className="font-medium text-brand-600 transition-colors hover:text-brand-700"
            >
              立即登录
            </Link>
          </p>
        </div>
      </main>
    </div>
  );
}
