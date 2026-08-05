import { redirect } from "next/navigation";
import { getPortalToken } from "@/lib/auth/portal-session";
import { PortalLoginClient } from "./portal-login-client";

export const dynamic = "force-dynamic";

export default async function PortalLoginPage({
  searchParams,
}: {
  searchParams: Promise<{ token?: string }>;
}) {
  if (await getPortalToken()) redirect("/portal");
  const params = await searchParams;
  return <PortalLoginClient initialToken={params.token ?? ""} />;
}
