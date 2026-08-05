import type { ReactNode } from "react";
import type { Env } from "@/lib/env-shared";
import { EnvProvider } from "./env-provider";
import { Sidebar } from "./sidebar";
import { Topbar, type TopbarUser } from "./topbar";

/**
 * 已登录应用外壳：侧边栏 + 顶栏（环境切换器）+ 内容区。
 * 仅服务端组件使用（凭据来自会话，不下发浏览器）。
 */
export function AppShell({
  user,
  env,
  onEnvChange,
  children,
}: {
  user: TopbarUser;
  env: Env;
  onEnvChange: (env: Env) => Promise<void>;
  children: ReactNode;
}) {
  return (
    <div className="min-h-dvh bg-canvas">
      <Sidebar />
      <div className="flex min-h-dvh flex-col lg:pl-60">
        <EnvProvider initialEnv={env} onChange={onEnvChange}>
          <Topbar user={user} />
          <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-8 sm:px-6">
            {children}
          </main>
        </EnvProvider>
      </div>
    </div>
  );
}
