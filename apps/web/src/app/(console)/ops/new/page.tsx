import { requireRole } from "@/lib/auth/rbac";
import { listRegions } from "@/lib/api/operator";
import type { Region } from "@/lib/api/operator";
import { NewProviderForm } from "./provider-form";

export const dynamic = "force-dynamic";

async function getRegions(): Promise<Region[]> {
  try {
    return await listRegions();
  } catch {
    return [];
  }
}

export default async function NewOpsPage() {
  await requireRole("operator");
  const regions = await getRegions();

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">新建 Provider</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          在测试环境创建一个服务提供商，创建完成后将获得沙箱环境与 API Key。
        </p>
      </header>
      <NewProviderForm regions={regions} />
    </div>
  );
}
