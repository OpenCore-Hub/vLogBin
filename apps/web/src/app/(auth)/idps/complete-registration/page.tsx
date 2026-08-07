import Link from "next/link";
import { redirect } from "next/navigation";
import { Logo } from "@/components/brand/logo";
import { Alert } from "@/components/ui/feedback";
import { getIdpFlow } from "@/lib/auth/login-flow";
import { getActiveIdentityProviders } from "@/lib/auth/zitadel-session";
import { IdpRegistrationForm } from "./idp-registration-form";

export const metadata = {
  title: "完善企业身份资料 · vLogBin",
};

export default async function CompleteRegistrationPage() {
  const flow = await getIdpFlow();
  if (
    !flow?.idpId ||
    !flow.idpIntentId ||
    !flow.idpIntentToken ||
    !flow.idpUserId ||
    !flow.email
  ) {
    redirect("/login?error=idp_state_expired");
  }

  let providerName = "企业身份源";
  try {
    const providers = await getActiveIdentityProviders(flow.organizationId);
    const provider = providers.find((candidate) => candidate.id === flow.idpId);
    if (!provider?.isCreationAllowed) {
      redirect("/idps/failure?error=creation_not_allowed");
    }
    providerName = provider.name;
  } catch {
    redirect("/login?error=idp_unavailable");
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
          <div className="mb-8 text-center">
            <h1 className="text-xl font-semibold text-foreground">
              完善企业身份资料
            </h1>
            <p className="mt-1 text-sm text-muted-foreground">
              通过 {providerName} 创建你的 vLogBin 账号
            </p>
          </div>
          {flow.email && (
            <div className="mb-4">
              <Alert variant="warning" title="邮箱由企业身份源提供">
                {flow.email}
              </Alert>
            </div>
          )}
          <IdpRegistrationForm
            givenName={flow.givenName ?? ""}
            familyName={flow.familyName ?? ""}
            email={flow.email}
          />
        </div>
      </main>
    </div>
  );
}
