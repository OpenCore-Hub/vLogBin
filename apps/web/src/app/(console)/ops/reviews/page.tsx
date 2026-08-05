import { requireRole } from "@/lib/auth/rbac";
import {
  listProviders,
  listRegions,
  listRiskReviews,
  listSupportSessions,
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

  const perProvider = await Promise.all(
    providers.map(async (provider) => {
      const [reviews, sessions] = await Promise.all([
        safeList<RiskReview>(() => listRiskReviews(provider.id)),
        safeList<SupportSession>(() => listSupportSessions(provider.id)),
      ]);
      return { provider, reviews, sessions };
    }),
  );
  const reviewRows = perProvider.flatMap(({ provider, reviews }) =>
    reviews[0] ? [{ provider, review: reviews[0] }] : [],
  );
  const supportSessions = perProvider.flatMap(({ sessions }) => sessions);
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
