import "server-only";

import { cookies } from "next/headers";
import { WORKSPACE_COOKIE } from "@/lib/env-shared";
import { listProviders, type Provider } from "@/lib/api/operator";

const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

/** 读取当前 workspace cookie（不做成员校验，由 provider 过滤兜底）。 */
export async function resolveWorkspaceId(): Promise<string | null> {
  const jar = await cookies();
  const id = jar.get(WORKSPACE_COOKIE)?.value;
  return id && UUID_PATTERN.test(id) ? id : null;
}

/** 当前 workspace 对应的 provider（cookie 优先，非法/失效回退第一个）。 */
export async function resolveWorkspaceProvider(): Promise<Provider | null> {
  const [workspaceId, providers] = await Promise.all([
    resolveWorkspaceId(),
    listProviders().catch(() => []),
  ]);
  return (
    providers.find((p) => p.id === workspaceId) ??
    providers[0] ??
    null
  );
}
