import type { ReactNode } from "react";
import { AppShell } from "@/components/console/shell";
import { QueryProvider } from "@/components/query-provider";
import { requireAuth, hasRole } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import { listMyWorkspaces } from "@/lib/api/operator";
import { resolveWorkspaceId } from "@/lib/workspace";
import { switchEnv, switchWorkspace } from "../actions";

export const dynamic = "force-dynamic";

export default async function ConsoleLayout({
  children,
}: {
  children: ReactNode;
}) {
  const session = await requireAuth();
  const env = await resolveEnv(session);
  const [workspaces, activeWorkspaceId] = await Promise.all([
    listMyWorkspaces().catch(() => []),
    resolveWorkspaceId(),
  ]);

  return (
    <QueryProvider>
      <AppShell
        user={{
          name: session.name || "User",
          email: session.email,
          isOperator: hasRole(session, "operator"),
        }}
        env={env}
        onEnvChange={switchEnv}
        workspaces={workspaces}
        activeWorkspaceId={activeWorkspaceId}
        onWorkspaceChange={switchWorkspace}
      >
        {children}
      </AppShell>
    </QueryProvider>
  );
}
