import type { Metadata } from "next";
import Link from "next/link";
import { Logo } from "@/components/brand/logo";
import { Alert } from "@/components/ui/feedback";
import { LinkButton } from "@/components/ui/button";
import { AlertIcon } from "@/components/ui/icons";

export const metadata: Metadata = {
  title: "认证失败 · vLogBin",
};

const MESSAGES: Record<string, { title: string; hint: string }> = {
  oidc_not_configured: {
    title: "OIDC 未配置",
    hint: "请设置 ZITADEL_URL 与 ZITADEL_CLIENT_ID 后重试。",
  },
  oidc_denied: {
    title: "登录被拒绝",
    hint: "身份提供方拒绝了本次登录请求。",
  },
  missing_code: {
    title: "缺少授权码",
    hint: "回调缺少 code 参数，请重新发起登录。",
  },
  state_mismatch: {
    title: "状态校验失败",
    hint: "登录状态已失效（可能已过期），请重新登录。",
  },
  missing_verifier: {
    title: "缺少 PKCE 校验码",
    hint: "请重新发起登录。",
  },
  token_exchange_failed: {
    title: "令牌交换失败",
    hint: "无法从身份提供方换取令牌，请稍后重试。",
  },
  invalid_id_token: {
    title: "身份令牌校验失败",
    hint: "无法验证身份令牌，请重新登录。",
  },
  missing_subject: {
    title: "缺少身份主体",
    hint: "身份令牌缺少必要声明，请联系管理员。",
  },
};

export default async function AuthErrorPage({
  searchParams,
}: {
  searchParams: Promise<{ error?: string; description?: string }>;
}) {
  const { error = "unknown", description } = await searchParams;
  const msg = MESSAGES[error] ?? {
    title: "认证失败",
    hint: description ?? "发生了未知错误，请重试。",
  };

  return (
    <div className="flex min-h-dvh flex-col items-center justify-center px-4">
      <div className="w-full max-w-md">
        <div className="mb-8 flex flex-col items-center gap-3 text-center">
          <Logo />
          <span className="flex size-11 items-center justify-center rounded-full bg-danger-soft text-danger">
            <AlertIcon size={20} />
          </span>
        </div>
        <Alert variant="danger" title={msg.title}>
          {msg.hint}
        </Alert>
        <div className="mt-5 flex justify-center gap-2">
          <LinkButton href="/login">重新登录</LinkButton>
          <Link
            href="/"
            className="inline-flex h-9.5 items-center rounded-md px-4 text-sm text-muted-foreground transition-colors hover:text-foreground"
          >
            返回首页
          </Link>
        </div>
      </div>
    </div>
  );
}
