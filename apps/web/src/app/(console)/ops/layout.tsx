import type { ReactNode } from "react";
import { AppShell } from "@/components/console/shell";
import { requireAuth, hasRole } from "@/lib/auth/rbac";
import { resolveEnv } from "@/lib/env";
import { switchEnv } from "../actions";

export const dynamic = "force-dynamic";

export default async function OpsLayout({ children }: { children: ReactNode }) {
  const session = await requireAuth();
  const env = await resolveEnv(session);

  return (
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
  );
}
