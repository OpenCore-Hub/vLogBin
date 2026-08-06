import { NextRequest, NextResponse } from "next/server";
import { exportAuditEvents } from "@/lib/api/operator";

function isoDaysAgo(days: number): string {
  return new Date(Date.now() - days * 24 * 60 * 60 * 1000).toISOString();
}

function toIso(value: string, endOfDay = false): string {
  if (!value) return endOfDay ? new Date().toISOString() : isoDaysAgo(7);
  if (value.includes("T")) {
    const parsed = new Date(value);
    return Number.isNaN(parsed.getTime())
      ? endOfDay
        ? new Date().toISOString()
        : isoDaysAgo(7)
      : parsed.toISOString();
  }
  const parsed = new Date(`${value}T${endOfDay ? "23:59:59" : "00:00:00"}`);
  return Number.isNaN(parsed.getTime())
    ? endOfDay
      ? new Date().toISOString()
      : isoDaysAgo(7)
    : parsed.toISOString();
}

export async function GET(request: NextRequest) {
  const search = request.nextUrl.searchParams;
  const providerId = search.get("provider_id") ?? "";
  const format = search.get("format") === "json" ? "json" : "csv";
  const from = toIso(search.get("from") || "");
  const to = toIso(search.get("to") || "", true);

  if (!providerId) {
    return NextResponse.json({ error: "missing provider_id" }, { status: 400 });
  }

  try {
    const upstream = await exportAuditEvents(providerId, {
      from,
      to,
      format,
      action: search.get("action") || undefined,
      actor_type: search.get("actor_type") || undefined,
      target_type: search.get("target_type") || undefined,
    });
    const body = Buffer.from(await upstream.arrayBuffer());
    return new NextResponse(body, {
      headers: {
        "Content-Type":
          format === "csv"
            ? "text/csv; charset=utf-8"
            : "application/json; charset=utf-8",
        "Content-Disposition": `attachment; filename="audit-${Date.now()}.${format}"`,
      },
    });
  } catch (err) {
    return NextResponse.json(
      { error: err instanceof Error ? err.message : "导出失败" },
      { status: 500 },
    );
  }
}
