import { requireAuth } from "@/lib/auth/rbac";
import {
  listReconciliationResults,
  type ReconciliationResult,
} from "@/lib/api/operator";
import { ReconciliationClient } from "./reconciliation-client";

export const dynamic = "force-dynamic";

export default async function ReconciliationPage() {
  await requireAuth();

  let results: ReconciliationResult[] = [];
  let loadError: string | null = null;
  try {
    results = await listReconciliationResults();
  } catch (err) {
    loadError = err instanceof Error ? err.message : "对账结果加载失败";
  }

  return <ReconciliationClient results={results} loadError={loadError} />;
}
