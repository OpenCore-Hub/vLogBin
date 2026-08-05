import { redirect } from "next/navigation";
import { getPortalDashboard } from "@/lib/api/portal";
import type { PortalDashboard } from "@/lib/api/types";
import { getPortalToken } from "@/lib/auth/portal-session";
import { PortalClient } from "./portal-client";

export const dynamic = "force-dynamic";

export default async function PortalPage() {
  const token = await getPortalToken();
  if (!token) redirect("/portal/login");

  let dashboard: PortalDashboard | null = null;
  let loadError: string | null = null;
  try {
    dashboard = await getPortalDashboard(token);
  } catch (err) {
    loadError = err instanceof Error ? err.message : "客户门户加载失败";
  }
  return <PortalClient dashboard={dashboard} loadError={loadError} />;
}
