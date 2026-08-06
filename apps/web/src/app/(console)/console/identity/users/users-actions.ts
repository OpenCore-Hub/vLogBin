"use server";

import { revalidatePath } from "next/cache";
import { z } from "zod";
import { requireAuth } from "@/lib/auth/rbac";
import {
  inviteWorkspaceMember,
  removeWorkspaceMember,
  updateWorkspaceMemberRole,
  type WorkspaceMembership,
} from "@/lib/api/operator";

const USERS_PATH = "/console/identity/users";

export interface MemberActionState {
  ok: boolean;
  error?: string;
  member?: WorkspaceMembership;
}

const memberInputSchema = z.object({
  workspaceId: z.string().trim().min(1, "缺少必要参数"),
  userSub: z.string().trim().min(1, "用户 subject 不能为空"),
  role: z.enum(
    ["provider_admin", "provider_developer", "provider_billing"],
    "角色无效",
  ),
});

function errorMessage(err: unknown): string {
  if (err instanceof Error && err.message) return err.message;
  return "发生未知错误，请稍后重试。";
}

function parseForm(formData: FormData) {
  return memberInputSchema.safeParse({
    workspaceId: formData.get("workspace_id"),
    userSub: formData.get("user_sub"),
    role: formData.get("role"),
  });
}

export async function inviteMemberAction(
  _prev: MemberActionState,
  formData: FormData,
): Promise<MemberActionState> {
  await requireAuth();
  const parsed = parseForm(formData);
  if (!parsed.success) {
    return { ok: false, error: parsed.error.issues[0]?.message ?? "输入无效" };
  }
  try {
    const member = await inviteWorkspaceMember(parsed.data.workspaceId, {
      user_sub: parsed.data.userSub,
      role: parsed.data.role,
    });
    if (!member) return { ok: false, error: "邀请失败：API 未返回成员" };
    revalidatePath(USERS_PATH);
    return { ok: true, member };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

export async function updateMemberRoleAction(
  _prev: MemberActionState,
  formData: FormData,
): Promise<MemberActionState> {
  await requireAuth();
  const parsed = parseForm(formData);
  if (!parsed.success) {
    return { ok: false, error: parsed.error.issues[0]?.message ?? "输入无效" };
  }
  try {
    const member = await updateWorkspaceMemberRole(
      parsed.data.workspaceId,
      parsed.data.userSub,
      parsed.data.role,
    );
    if (!member) return { ok: false, error: "更新失败：API 未返回成员" };
    revalidatePath(USERS_PATH);
    return { ok: true, member };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

export async function removeMemberAction(
  _prev: MemberActionState,
  formData: FormData,
): Promise<MemberActionState> {
  await requireAuth();
  const workspaceId = String(formData.get("workspace_id") ?? "").trim();
  const userSub = String(formData.get("user_sub") ?? "").trim();
  if (!workspaceId || !userSub) return { ok: false, error: "缺少必要参数" };
  try {
    await removeWorkspaceMember(workspaceId, userSub);
    revalidatePath(USERS_PATH);
    return { ok: true };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}
