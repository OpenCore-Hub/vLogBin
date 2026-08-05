import type { ReactNode } from "react";
import { AppShell } from "@/components/console/shell";
import { QueryProvider } from "@/components/query-provider";
import { requireAuth, hasRole } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import { switchEnv } from "../actions";

export const dynamic = "force-dynamic";

export default async function ConsoleLayout({
  children,
}: {
  children: ReactNode;
}) {
  const session = await requireAuth();
  const env = await resolveEnv(session);

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
      >
        {children}
      </AppShell>
    </QueryProvider>
  );
}
