import { requireAuth } from "@/lib/auth/rbac";
import {
  listMyWorkspaces,
  listWorkspaceMembers,
  type WorkspaceMembership,
} from "@/lib/api/operator";
import { UsersClient } from "./users-client";

export const dynamic = "force-dynamic";

export default async function UsersPage() {
  await requireAuth();

  const workspaces = await listMyWorkspaces().catch(() => []);
  const workspace = workspaces[0] ?? null;

  let members: WorkspaceMembership[] = [];
  let loadError: string | null = null;
  if (workspace) {
    try {
      members = await listWorkspaceMembers(workspace.id);
    } catch (err) {
      loadError = err instanceof Error ? err.message : "成员列表加载失败";
    }
  }

  return (
    <UsersClient
      workspaceId={workspace?.id ?? null}
      workspaceName={workspace?.name ?? null}
      members={members}
      loadError={loadError}
    />
  );
}
