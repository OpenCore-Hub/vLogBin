import { requireRole } from "@/lib/auth/rbac";
import {
  listCellMigrations,
  listCells,
  listFailovers,
  listProviders,
  listRegions,
  type Cell,
  type CellFailover,
  type CellMigration,
  type Provider,
  type Region,
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

export default async function OpsCellsPage() {
  await requireRole("operator");
  const [providers, regions, cells] = await Promise.all([
    listProviders().catch(() => [] as Provider[]),
    listRegions().catch(() => [] as Region[]),
    listCells().catch(() => [] as Cell[]),
  ]);

  const perProvider = await Promise.all(
    providers.map(async (provider) => {
      const [failovers, migrations] = await Promise.all([
        safeList<CellFailover>(() => listFailovers(provider.id)),
        safeList<CellMigration>(() => listCellMigrations(provider.id)),
      ]);
      return { failovers, migrations };
    }),
  );

  return (
    <OpsClient
      providers={providers}
      regions={regions}
      reviewRows={[]}
      awaitingReviews={[]}
      supportSessions={[]}
      cells={cells}
      failovers={perProvider.flatMap(({ failovers }) => failovers)}
      migrations={perProvider.flatMap(({ migrations }) => migrations)}
      error={null}
    />
  );
}
