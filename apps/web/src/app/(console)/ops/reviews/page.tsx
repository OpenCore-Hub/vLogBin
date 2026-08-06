import { requireRole } from "@/lib/auth/rbac";
import {
  listProviders,
  listRegions,
  listLatestRiskReviews,
  listAllSupportSessions,
  type Provider,
  type Region,
  type RiskReview,
  type SupportSession,
} from "@/lib/api/operator";
import { OpsClient } from "../ops-client";

export const dynamic = "force-dynamic";

async function safeList<T>(fn: () => Promise<T[]>): Promise<T[]> {
  try {
    return await fn();
  } catch {
    return [];
  }
}

export default async function OpsReviewsPage() {
  await requireRole("operator");
  const [providers, regions] = await Promise.all([
    listProviders().catch(() => [] as Provider[]),
    listRegions().catch(() => [] as Region[]),
  ]);

  const [reviews, supportSessions] = await Promise.all([
    safeList<RiskReview>(() => listLatestRiskReviews()),
    safeList<SupportSession>(() => listAllSupportSessions()),
  ]);
  const providerByID = new Map(providers.map((p) => [p.id, p]));
  const reviewRows = reviews
    .map((review) => {
      const provider = providerByID.get(review.provider_id);
      return provider ? { provider, review } : null;
    })
    .filter(
      (row): row is { provider: Provider; review: RiskReview } => row !== null,
    );
  const awaitingReviews = providers.filter(
    (p) =>
      p.lifecycle_state === "LIVE_REVIEW" &&
      !reviewRows.some((row) => row.provider.id === p.id && row.review.decision === "approved"),
  );

  return (
    <OpsClient
      providers={providers}
      regions={regions}
      reviewRows={reviewRows}
      awaitingReviews={awaitingReviews}
      supportSessions={supportSessions}
      cells={[]}
      failovers={[]}
      migrations={[]}
      error={null}
    />
  );
}
