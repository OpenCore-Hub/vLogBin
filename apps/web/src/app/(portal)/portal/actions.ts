"use server";

import { redirect } from "next/navigation";
import { validatePortalSession } from "@/lib/api/portal";
import {
  clearPortalToken,
  setPortalToken,
} from "@/lib/auth/portal-session";

export interface PortalActionState {
  ok: boolean;
  error?: string;
}

export async function loginPortalAction(
  _prev: PortalActionState,
  formData: FormData,
): Promise<PortalActionState> {
  const token = String(formData.get("token") ?? "").trim();
  if (!token) return { ok: false, error: "请输入门户邀请 Token" };
  try {
    const session = await validatePortalSession(token);
    if (!session.valid || !session.customer_external_id) {
      return { ok: false, error: "门户邀请 Token 无效或已过期" };
    }
    await setPortalToken(token);
    redirect("/portal");
  } catch (err) {
    return {
      ok: false,
      error: err instanceof Error ? err.message : "门户 Token 验证失败",
    };
  }
}

export async function logoutPortalAction(): Promise<void> {
  await clearPortalToken();
  redirect("/portal/login");
}
