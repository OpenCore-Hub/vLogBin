import Link from "next/link";
import { Logo } from "@/components/brand/logo";
import { Alert } from "@/components/ui/feedback";
import { LinkButton } from "@/components/ui/button";
import { AlertIcon } from "@/components/ui/icons";

export const metadata = {
  title: "企业身份登录失败 · vLogBin",
};

export default async function IdpFailurePage({
  searchParams,
}: {
  searchParams: Promise<{ error?: string; description?: string }>;
}) {
  const { error, description } = await searchParams;
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
            <span className="flex size-14 items-center justify-center rounded-2xl bg-danger-soft text-danger">
              <AlertIcon size={22} />
            </span>
            <h1 className="text-xl font-semibold text-foreground">
              企业身份登录未完成
            </h1>
            <p className="text-sm text-muted-foreground">
              你可以返回登录页重试，或联系管理员检查企业身份源配置。
            </p>
          </div>
          <Alert variant="danger" title={error || "idp_failed"}>
            {description || "企业身份源未返回可用的登录结果。"}
          </Alert>
          <div className="mt-4">
            <LinkButton href="/login" className="w-full">
              返回登录
            </LinkButton>
          </div>
        </div>
      </main>
    </div>
  );
}
