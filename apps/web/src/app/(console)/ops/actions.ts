"use server";

import { revalidatePath } from "next/cache";
import { requireRole } from "@/lib/auth/rbac";
import {
  activateProvider,
  assignProviderCell,
  createCell,
  createProvider,
  replayWebhookDelivery,
  revokeCredential,
  submitRiskReview,
  transitionLifecycle,
  updateCellStatus,
  type RiskReview,
  type LifecycleTarget,
} from "@/lib/api/operator";
import {
  activateProviderInputSchema,
  createProviderInputSchema,
  lifecycleReasonSchema,
  lifecycleTargetSchema,
} from "@/lib/validate";

export interface OpActionState {
  ok: boolean;
  error?: string;
  apiKey?: string;
  providerId?: string;
  review?: RiskReview;
}

function errorMessage(err: unknown): string {
  if (err instanceof Error && err.message) return err.message;
  return "发生未知错误，请稍后重试。";
}

/** 创建 Provider（细粒度 RBAC：仅 operator）。 */
export async function createProviderAction(
  _prev: OpActionState,
  formData: FormData,
): Promise<OpActionState> {
  await requireRole("operator");

  const parsed = createProviderInputSchema.safeParse({
    slug: formData.get("slug"),
    name: formData.get("name"),
    home_region_code: formData.get("home_region_code"),
  });
  if (!parsed.success) {
    return { ok: false, error: parsed.error.issues[0]?.message ?? "输入无效" };
  }

  try {
    const result = await createProvider({
      slug: parsed.data.slug,
      name: parsed.data.name,
      home_region_code: parsed.data.home_region_code,
    });
    revalidatePath("/ops");
    revalidatePath("/console");
    return {
      ok: true,
      apiKey: result.apiKey ?? undefined,
      providerId: result.provider?.id ?? undefined,
    };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

/** 激活 Provider（资源分配）：选择区域 → 分配 Cell、创建测试环境并签发 API Key。 */
export async function activateProviderAction(
  _prev: OpActionState,
  formData: FormData,
): Promise<OpActionState> {
  await requireRole("operator");

  const parsed = activateProviderInputSchema.safeParse({
    provider_id: formData.get("provider_id"),
    home_region_code: formData.get("home_region_code"),
    reason: formData.get("reason"),
  });
  if (!parsed.success) {
    return { ok: false, error: parsed.error.issues[0]?.message ?? "输入无效" };
  }

  try {
    const result = await activateProvider(
      parsed.data.provider_id,
      parsed.data.home_region_code,
      parsed.data.reason,
    );
    revalidatePath("/ops");
    revalidatePath(`/ops/${parsed.data.provider_id}`);
    revalidatePath("/console");
    return {
      ok: true,
      apiKey: result.apiKey ?? undefined,
      providerId: result.provider?.id ?? undefined,
    };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

/** 吊销 Provider 的 API 密钥（细粒度 RBAC：仅 operator）。吊销即时生效，动作记入审计轨迹。 */
export async function revokeCredentialAction(
  _prev: OpActionState,
  formData: FormData,
): Promise<OpActionState> {
  await requireRole("operator");

  const providerId = String(formData.get("provider_id") ?? "").trim();
  const credentialId = String(formData.get("credential_id") ?? "").trim();
  if (!providerId || !credentialId) {
    return { ok: false, error: "缺少必要参数" };
  }

  try {
    await revokeCredential(providerId, credentialId);
    revalidatePath(`/ops/${providerId}`);
    revalidatePath("/console");
    return { ok: true, providerId };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

/** 重放终态（dead_letter / failed）的 webhook 投递（细粒度 RBAC：仅 operator）。 */
export async function replayWebhookDeliveryAction(
  _prev: OpActionState,
  formData: FormData,
): Promise<OpActionState> {
  await requireRole("operator");

  const providerId = String(formData.get("provider_id") ?? "").trim();
  const deliveryId = String(formData.get("delivery_id") ?? "").trim();
  if (!providerId || !deliveryId) {
    return { ok: false, error: "缺少必要参数" };
  }

  try {
    await replayWebhookDelivery(providerId, deliveryId, "operator");
    revalidatePath(`/ops/${providerId}`);
    return { ok: true, providerId };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

/** 执行生命周期迁移（与 API 侧状态机一致，非法目标由服务端拒绝）。 */
export async function transitionLifecycleAction(
  _prev: OpActionState,
  formData: FormData,
): Promise<OpActionState> {
  await requireRole("operator");

  const providerId = String(formData.get("provider_id") ?? "").trim();
  const toRaw = String(formData.get("to") ?? "").trim();
  if (!providerId) return { ok: false, error: "缺少 Provider ID" };

  const parsed = lifecycleTargetSchema.safeParse(toRaw);
  if (!parsed.success) {
    return { ok: false, error: `非法生命周期目标：${toRaw}` };
  }

  const reason = formData.get("reason");
  const parsedReason = lifecycleReasonSchema.safeParse(
    reason === null ? undefined : String(reason),
  );
  if (!parsedReason.success) {
    return { ok: false, error: parsedReason.error.issues[0]?.message ?? "操作原因无效" };
  }

  try {
    const result = await transitionLifecycle(
      providerId,
      parsed.data as LifecycleTarget,
      parsedReason.data,
    );
    revalidatePath("/ops");
    revalidatePath(`/ops/${providerId}`);
    revalidatePath("/console");
    return {
      ok: true,
      apiKey: result.apiKey ?? undefined,
      providerId,
    };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

/** 提交 Live Provider 风险审核（8 项 go-live checklist + risk_score）。 */
export async function submitRiskReviewAction(
  _prev: OpActionState,
  formData: FormData,
): Promise<OpActionState> {
  await requireRole("operator");
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const riskScore = Number(formData.get("risk_score"));
  const decisionRaw = String(formData.get("decision") ?? "");
  if (!providerId || !["approved", "rejected"].includes(decisionRaw)) {
    return { ok: false, error: "缺少必要参数" };
  }
  if (!Number.isFinite(riskScore) || riskScore < 0 || riskScore > 100) {
    return { ok: false, error: "风险评分必须在 0-100 之间" };
  }
  const checks = Object.fromEntries(
    [
      "email_and_company_domain",
      "tos_dpa",
      "custom_domain_ownership",
      "payment_tax_connection",
      "webhook_destination",
      "initial_quota",
      "security_contact",
    ].map((key) => [key, formData.get(`check_${key}`) === "on"]),
  ) as Record<string, boolean>;

  try {
    const review = await submitRiskReview(providerId, {
      risk_score: riskScore,
      checks,
      decision: decisionRaw as "approved" | "rejected",
      reason: String(formData.get("reason") ?? "").trim() || undefined,
      reviewed_by: String(formData.get("reviewed_by") ?? "").trim() || "operator",
    });
    if (!review) return { ok: false, error: "提交失败：API 未返回审核记录" };
    revalidatePath("/ops");
    revalidatePath(`/ops/${providerId}`);
    return { ok: true, providerId, review };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

/** 创建 Cell（区域 / 类型 / 状态 / 容量限制）。 */
export async function createCellAction(
  _prev: OpActionState,
  formData: FormData,
): Promise<OpActionState> {
  await requireRole("operator");
  const regionId = String(formData.get("region_id") ?? "").trim();
  const code = String(formData.get("code") ?? "").trim();
  const cellType = String(formData.get("cell_type") ?? "");
  const status = String(formData.get("status") ?? "");
  if (!regionId || !code || !["shared", "dedicated"].includes(cellType)) {
    return { ok: false, error: "缺少必要参数" };
  }
  try {
    const cell = await createCell({
      region_id: regionId,
      code,
      cell_type: cellType as "shared" | "dedicated",
      status: (status === "draining" || status === "inactive" ? status : "active") as "active" | "draining" | "inactive",
      capacity_limits: {},
    });
    if (!cell) return { ok: false, error: "创建失败：API 未返回 Cell" };
    revalidatePath("/ops");
    return { ok: true };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

/** 更新 Cell 状态（active / draining / inactive）。 */
export async function updateCellStatusAction(
  _prev: OpActionState,
  formData: FormData,
): Promise<OpActionState> {
  await requireRole("operator");
  const cellId = String(formData.get("cell_id") ?? "").trim();
  const status = String(formData.get("status") ?? "");
  if (!cellId || !["active", "draining", "inactive"].includes(status)) {
    return { ok: false, error: "缺少必要参数" };
  }
  try {
    await updateCellStatus(cellId, status as "active" | "draining" | "inactive");
    revalidatePath("/ops");
    return { ok: true };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

/** 将 Provider 分配到指定 Cell。 */
export async function assignProviderCellAction(
  _prev: OpActionState,
  formData: FormData,
): Promise<OpActionState> {
  await requireRole("operator");
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const cellId = String(formData.get("cell_id") ?? "").trim();
  if (!providerId || !cellId) return { ok: false, error: "缺少必要参数" };
  try {
    await assignProviderCell(providerId, cellId);
    revalidatePath("/ops");
    return { ok: true, providerId };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}
