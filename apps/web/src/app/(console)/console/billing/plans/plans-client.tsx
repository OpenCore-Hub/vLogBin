"use client";

import {
  Fragment,
  startTransition,
  useActionState,
  useEffect,
  useRef,
  useState,
} from "react";
import { useRouter } from "next/navigation";
import type {
  Metric,
  PlanCollection,
  PlanDetail,
  PlanInput,
  Price,
} from "@/lib/api/operator";
import type { Env } from "@/lib/env-shared";
import { formatMoney } from "@/lib/format";
import { Button, LinkButton } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/field";
import { Dialog, DropdownMenu, ConfirmDialog } from "@/components/ui/overlay";
import { EmptyState, ErrorState, Alert, SuccessPanel } from "@/components/ui/feedback";
import { Badge, EnvBadge } from "@/components/ui/badge";
import { DataTable, type DataTableColumn } from "@/components/ui/data-table";
import { useEnv } from "@/components/console/env-provider";
import { useToast } from "@/components/ui/toast";
import {
  ArrowRightIcon,
  EditIcon,
  KebabIcon,
  PackageIcon,
  PlusIcon,
  TrashIcon,
} from "@/components/ui/icons";
import {
  createPlanAction,
  deletePlanAction,
  updatePlanAction,
  type PlanActionState,
} from "./plans-actions";

const initialState: PlanActionState = { ok: false };
const AMOUNT_RE = /^\d+(\.\d{1,2})?$/;
const CODE_RE = /^[a-z0-9](?:[a-z0-9_-]{0,61}[a-z0-9])?$/;

type ChargeModel = "fixed" | "per_unit" | "tiered";
type Interval = "weekly" | "monthly" | "yearly";

type PriceRow = {
  id: string;
  chargeModel: ChargeModel;
  metricCode: string;
  amount: string;
  tiers: TierRow[];
};

type TierRow = {
  id: string;
  from: string;
  to: string; // 空 = 开放区间
  unitAmount: string;
};

type PlanFormModel = {
  code: string;
  name: string;
  interval: Interval;
  currency: string;
  prices: PriceRow[];
};

function uid(): string {
  return Math.random().toString(36).slice(2, 10);
}

function newPriceRow(chargeModel: ChargeModel = "fixed"): PriceRow {
  return { id: uid(), chargeModel, metricCode: "", amount: "", tiers: [] };
}

function newTierRow(): TierRow {
  return { id: uid(), from: "", to: "", unitAmount: "" };
}

function yuanToCents(value: string): number {
  if (!AMOUNT_RE.test(value)) return -1;
  const [whole = "0", frac = ""] = value.split(".");
  return Number(whole) * 100 + Number(frac.padEnd(2, "0") || "0");
}

function centsToYuan(cents: unknown): string {
  const n = typeof cents === "number" ? cents : Number(cents ?? 0);
  if (!Number.isFinite(n)) return "";
  return (n / 100).toFixed(2).replace(/\.?0+$/, "");
}

function priceProps(p: Price): Record<string, unknown> {
  return p.properties && typeof p.properties === "object"
    ? (p.properties as Record<string, unknown>)
    : {};
}

function numProp(props: Record<string, unknown>, key: string): number | null {
  const value = props[key];
  return typeof value === "number" ? value : null;
}

function priceToRow(p: Price, index: number): PriceRow {
  const props = priceProps(p);
  const base = {
    id: `${p.id || index}-${uid()}`,
    chargeModel: (p.charge_model === "fixed" ||
    p.charge_model === "per_unit" ||
    p.charge_model === "tiered"
      ? p.charge_model
      : "fixed") as ChargeModel,
    metricCode: p.metric_code ?? "",
    amount: "",
    tiers: [] as TierRow[],
  };
  if (p.charge_model === "fixed") {
    return { ...base, amount: centsToYuan(props.amount_cents) };
  }
  if (p.charge_model === "per_unit") {
    return { ...base, amount: centsToYuan(props.unit_amount_cents) };
  }
  const tiers = Array.isArray(props.tiers) ? props.tiers : [];
  return {
    ...base,
    tiers: tiers.map((t, i) => ({
      id: `${i}-${uid()}`,
      from: String(t?.from_value ?? ""),
      to: t?.to_value == null ? "" : String(t.to_value),
      unitAmount: centsToYuan(t?.unit_amount_cents),
    })),
  };
}

function emptyForm(): PlanFormModel {
  return {
    code: "",
    name: "",
    interval: "monthly",
    currency: "USD",
    prices: [newPriceRow("fixed")],
  };
}

function formFromDetail(detail: PlanDetail): PlanFormModel {
  const plan = detail.plan;
  const interval =
    plan.interval === "weekly" ||
    plan.interval === "monthly" ||
    plan.interval === "yearly"
      ? plan.interval
      : "monthly";
  return {
    code: plan.code,
    name: plan.name,
    interval,
    currency: plan.currency,
    prices:
      detail.prices.length > 0
        ? detail.prices.map(priceToRow)
        : [newPriceRow("fixed")],
  };
}

function buildPlanInput(form: PlanFormModel): PlanInput {
  const currency = form.currency.trim().toUpperCase();
  return {
    code: form.code.trim(),
    name: form.name.trim(),
    interval: form.interval,
    currency,
    prices: form.prices.map((row) => {
      if (row.chargeModel === "fixed") {
        return {
          charge_model: "fixed",
          properties: { amount_cents: yuanToCents(row.amount), currency },
        };
      }
      if (row.chargeModel === "per_unit") {
        return {
          charge_model: "per_unit",
          metric_code: row.metricCode,
          properties: { unit_amount_cents: yuanToCents(row.amount), currency },
        };
      }
      return {
        charge_model: "tiered",
        metric_code: row.metricCode,
        properties: {
          tiers: row.tiers.map((t) => ({
            from_value: Number(t.from),
            to_value: t.to === "" ? null : Number(t.to),
            unit_amount_cents: yuanToCents(t.unitAmount),
          })),
        },
      };
    }),
  };
}

function validateTiers(tiers: TierRow[]): string {
  if (tiers.length === 0) return "至少需要一个区间";
  for (let i = 0; i < tiers.length; i++) {
    const t = tiers[i];
    if (!/^\d+$/.test(t.from)) return `区间 ${i + 1}：from 必须为非负整数`;
    if (t.to !== "" && !/^\d+$/.test(t.to)) {
      return `区间 ${i + 1}：to 必须为非负整数`;
    }
    if (!AMOUNT_RE.test(t.unitAmount)) return `区间 ${i + 1}：单价金额无效`;
    if (i === 0 && Number(t.from) !== 0) return "第一个区间必须从 0 开始";
    if (t.to !== "" && Number(t.to) <= Number(t.from)) {
      return `区间 ${i + 1}：to 必须大于 from`;
    }
    if (i > 0) {
      const prev = tiers[i - 1];
      if (prev.to === "" || Number(prev.to) !== Number(t.from)) {
        return `区间 ${i + 1}：必须与上一区间连续`;
      }
    }
  }
  if (tiers[tiers.length - 1].to !== "") return "最后一个区间必须开放（不填上限）";
  return "";
}

function validateForm(form: PlanFormModel): Record<string, string> {
  const errors: Record<string, string> = {};
  if (!form.code.trim()) errors["plan-code"] = "套餐代码不能为空";
  else if (!CODE_RE.test(form.code.trim())) {
    errors["plan-code"] = "代码只能包含小写字母、数字、中划线和下划线，且不能以中划线/下划线开头结尾";
  }
  if (!form.name.trim()) errors["plan-name"] = "套餐名称不能为空";
  if (!/^[A-Za-z]{3}$/.test(form.currency.trim())) {
    errors["plan-currency"] = "货币代码必须为 3 位字母";
  }
  if (form.prices.length === 0) errors["prices"] = "至少需要一个价格";
  form.prices.forEach((row, i) => {
    if (row.chargeModel !== "fixed" && !row.metricCode) {
      errors[`price-${i}-metric`] = "请选择计费指标";
    }
    if (row.chargeModel !== "tiered" && !AMOUNT_RE.test(row.amount)) {
      errors[`price-${i}-amount`] = "请输入非负金额，最多两位小数";
    }
    if (row.chargeModel === "tiered") {
      const tierErr = validateTiers(row.tiers);
      if (tierErr) errors[`price-${i}-tiers`] = tierErr;
    }
  });
  return errors;
}

function focusFirstError(errors: Record<string, string>) {
  const first = Object.keys(errors)[0];
  if (first) {
    requestAnimationFrame(() => document.getElementById(first)?.focus());
  }
}

const INTERVAL_LABEL: Record<Interval, string> = {
  weekly: "每周",
  monthly: "每月",
  yearly: "每年",
};

function priceSummary(detail: PlanDetail): string {
  const { plan, prices } = detail;
  if (prices.length === 0) return "—";
  const first = prices[0];
  const props = priceProps(first);
  if (first.charge_model === "fixed") {
    return `${formatMoney(numProp(props, "amount_cents"), plan.currency)} / ${INTERVAL_LABEL[plan.interval as Interval] ?? plan.interval}`;
  }
  if (first.charge_model === "per_unit") {
    return `${formatMoney(numProp(props, "unit_amount_cents"), plan.currency)} / ${first.metric_code ?? "指标"}`;
  }
  return `阶梯 · ${first.metric_code ?? "指标"}`;
}

export function PlansClient({
  providerId,
  env,
  collection,
  loadError,
}: {
  providerId: string | null;
  env: Env;
  collection: PlanCollection;
  loadError: string | null;
}) {
  const router = useRouter();
  const { env: activeEnv } = useEnv();
  const prevEnv = useRef(env);
  const [createOpen, setCreateOpen] = useState(false);
  const [createNonce, setCreateNonce] = useState(0);
  const [editing, setEditing] = useState<PlanDetail | null>(null);
  const [deleting, setDeleting] = useState<PlanDetail | null>(null);

  useEffect(() => {
    if (prevEnv.current !== activeEnv) {
      prevEnv.current = activeEnv;
      router.refresh();
    }
  }, [activeEnv, router]);

  function openCreate() {
    setCreateNonce((n) => n + 1);
    setCreateOpen(true);
  }

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">套餐</h1>
          <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
            定义订阅价格模型：固定价格、按量或阶梯计费。改动先进入 draft 目录版本，
            发布后对新的订阅生效。当前环境为{" "}
            {env === "test" ? "测试环境（沙箱）" : "生产环境（真实订阅生效）"}。
          </p>
        </div>
        {providerId && (
          <Button onClick={openCreate}>
            <PlusIcon size={16} aria-hidden="true" />
            创建套餐
          </Button>
        )}
      </header>

      {loadError ? (
        <ErrorState
          title="套餐列表加载失败"
          description={loadError}
          action={
            <Button variant="outline" onClick={() => router.refresh()}>
              重试
            </Button>
          }
        />
      ) : !providerId ? (
        <EmptyState
          icon={<PackageIcon size={20} aria-hidden="true" />}
          title="还没有可管理的 workspace"
          description="先创建并激活 Provider，获得测试环境后即可创建第一个套餐。"
          action={
            <LinkButton href="/ops" variant="primary" prefetch={false}>
              前往 Provider
              <ArrowRightIcon size={16} aria-hidden="true" />
            </LinkButton>
          }
        />
      ) : collection.plans.length === 0 ? (
        <EmptyState
          icon={<PackageIcon size={20} aria-hidden="true" />}
          title="还没有套餐"
          description={`在${env === "test" ? "测试环境" : "生产环境"}创建第一个套餐，定义订阅价格后即可进入客户订阅流程。`}
          action={
            <Button onClick={openCreate}>
              <PlusIcon size={16} aria-hidden="true" />
              创建第一个套餐
            </Button>
          }
        />
      ) : (
        <PlanTable
          plans={collection.plans}
          env={env}
          onEdit={setEditing}
          onDelete={setDeleting}
        />
      )}

      {providerId && (
        <>
          <PlanFormDialog
            key={`create-${createNonce}`}
            mode="create"
            open={createOpen}
            onOpenChange={setCreateOpen}
            providerId={providerId}
            env={env}
            metrics={collection.metrics}
          />

          {editing && (
            <PlanFormDialog
              key={`edit-${editing.plan.id}`}
              mode="edit"
              open
              onOpenChange={(open) => {
                if (!open) setEditing(null);
              }}
              providerId={providerId}
              env={env}
              metrics={collection.metrics}
              initial={editing}
            />
          )}

          {deleting && (
            <DeletePlanDialog
              key={`delete-${deleting.plan.id}`}
              open
              onOpenChange={(open) => {
                if (!open) setDeleting(null);
              }}
              plan={deleting}
              providerId={providerId}
              env={env}
            />
          )}
        </>
      )}
    </div>
  );
}

/* ---------------- 套餐列表（DataTable + URL 筛选） ---------------- */
function PlanTable({
  plans,
  env,
  onEdit,
  onDelete,
}: {
  plans: PlanDetail[];
  env: Env;
  onEdit: (plan: PlanDetail) => void;
  onDelete: (plan: PlanDetail) => void;
}) {
  const columns: DataTableColumn<PlanDetail>[] = [
    {
      key: "name",
      header: "套餐",
      sortable: true,
      sortValue: (d) => d.plan.name,
      cell: (d) => (
        <span>
          <span className="block font-medium text-foreground">{d.plan.name}</span>
          <span className="block font-mono text-xs text-muted-foreground">
            {d.plan.code}
          </span>
        </span>
      ),
    },
    {
      key: "price",
      header: "价格",
      cell: (d) => (
        <span>
          <span className="font-mono text-xs text-foreground tabular-nums">
            {priceSummary(d)}
          </span>
          {d.prices.length > 1 && (
            <span className="ml-2 text-xs text-muted-foreground">
              +{d.prices.length - 1} 项
            </span>
          )}
        </span>
      ),
    },
    {
      key: "interval",
      header: "计费周期",
      cell: (d) => (
        <Badge variant="brand">
          {INTERVAL_LABEL[d.plan.interval as Interval] ?? d.plan.interval}
        </Badge>
      ),
    },
    {
      key: "currency",
      header: "货币",
      cell: (d) => (
        <span>
          <span className="font-mono text-xs text-muted-foreground">
            {d.plan.currency}
          </span>
          <EnvBadge env={env} />
        </span>
      ),
    },
    {
      key: "actions",
      header: "",
      className: "text-right",
      cell: (d) => (
        <DropdownMenu
          align="end"
          triggerLabel={`${d.plan.name} 操作`}
          trigger={
            <span className="inline-flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-surface-2 hover:text-foreground">
              <KebabIcon size={16} />
            </span>
          }
          items={[
            {
              type: "item",
              label: (
                <span className="inline-flex items-center gap-2">
                  <EditIcon size={14} aria-hidden="true" />
                  编辑套餐
                </span>
              ),
              onSelect: () => onEdit(d),
            },
            { type: "separator" },
            {
              type: "item",
              label: (
                <span className="inline-flex items-center gap-2">
                  <TrashIcon size={14} aria-hidden="true" />
                  删除套餐
                </span>
              ),
              danger: true,
              onSelect: () => onDelete(d),
            },
          ]}
        />
      ),
    },
  ];

  return (
    <DataTable
      data={plans}
      columns={columns}
      rowKey={(d) => d.plan.id}
      searchKeys={(d) => [d.plan.name, d.plan.code, d.plan.currency]}
      defaultSort={{ key: "name", dir: "asc" }}
      emptyLabel="暂无套餐"
    />
  );
}

/* ---------------- 创建 / 编辑套餐 ---------------- */
function PlanFormDialog({
  open,
  onOpenChange,
  mode,
  providerId,
  env,
  metrics,
  initial,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode: "create" | "edit";
  providerId: string;
  env: Env;
  metrics: Metric[];
  initial?: PlanDetail;
}) {
  const router = useRouter();
  const action = mode === "create" ? createPlanAction : updatePlanAction;
  const [state, formAction, pending] = useActionState(action, initialState);
  const [form, setForm] = useState<PlanFormModel>(() =>
    mode === "edit" && initial ? formFromDetail(initial) : emptyForm(),
  );
  const [errors, setErrors] = useState<Record<string, string>>({});

  useEffect(() => {
    if (state.ok) router.refresh();
  }, [state.ok, router]);

  function setPrice(index: number, next: PriceRow) {
    setForm((f) => ({
      ...f,
      prices: f.prices.map((p, i) => (i === index ? next : p)),
    }));
  }

  function submit(formData: FormData) {
    const next = validateForm(form);
    setErrors(next);
    if (Object.keys(next).length > 0) {
      focusFirstError(next);
      return;
    }
    const payload = buildPlanInput(form);
    formData.set("provider_id", providerId);
    formData.set("env", env);
    formData.set("code", payload.code);
    formData.set("payload", JSON.stringify(payload));
    formAction(formData);
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={mode === "create" ? "创建套餐" : `编辑套餐 · ${initial?.plan.name ?? ""}`}
      description={
        mode === "create"
          ? `套餐将写入当前 draft 目录版本，发布后对${env === "test" ? "测试环境" : "生产环境"}的新订阅生效。`
          : "修改会保留套餐 ID；若已有已发布版本，改动会 staged 到新的 draft，不改变线上订阅。"
      }
      size="lg"
    >
      {state.ok && state.detail ? (
        <div className="space-y-4">
          <SuccessPanel
            title={mode === "create" ? "套餐创建成功" : "套餐已更新"}
            description={`${state.detail.plan.name}（${state.detail.plan.code}）已写入 draft 目录版本。`}
          >
            <div className="mt-3 rounded-md border border-border bg-surface-2 p-3 font-mono text-xs text-foreground">
              {priceSummary(state.detail)}
            </div>
          </SuccessPanel>
          <div className="flex flex-wrap items-center justify-end gap-3">
            <Button variant="ghost" onClick={() => onOpenChange(false)}>
              返回套餐列表
            </Button>
            <LinkButton
              href="/console/billing/customers"
              variant="primary"
              prefetch={false}
              onClick={() => onOpenChange(false)}
            >
              继续创建客户
              <ArrowRightIcon size={16} aria-hidden="true" />
            </LinkButton>
          </div>
        </div>
      ) : (
        <form action={submit} className="space-y-5" noValidate>
          {state.error && (
            <Alert variant="danger" title={mode === "create" ? "创建失败" : "更新失败"}>
              {state.error}
            </Alert>
          )}

          <div className="grid gap-4 sm:grid-cols-2">
            <Field
              label="套餐代码"
              htmlFor="plan-code"
              hint={mode === "edit" ? "代码创建后不可修改" : "小写字母、数字、中划线/下划线"}
              error={errors["plan-code"]}
            >
              <Input
                id="plan-code"
                value={form.code}
                onChange={(e) => setForm((f) => ({ ...f, code: e.target.value }))}
                placeholder="pro"
                autoComplete="off"
                spellCheck={false}
                disabled={mode === "edit"}
                autoFocus
                invalid={Boolean(errors["plan-code"])}
              />
            </Field>
            <Field label="套餐名称" htmlFor="plan-name" error={errors["plan-name"]}>
              <Input
                id="plan-name"
                value={form.name}
                onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                placeholder="专业版"
                autoComplete="off"
                invalid={Boolean(errors["plan-name"])}
              />
            </Field>
            <Field label="计费周期" htmlFor="plan-interval" error={errors["plan-interval"]}>
              <Select
                id="plan-interval"
                value={form.interval}
                onChange={(e) =>
                  setForm((f) => ({ ...f, interval: e.target.value as Interval }))
                }
              >
                <option value="weekly">每周</option>
                <option value="monthly">每月</option>
                <option value="yearly">每年</option>
              </Select>
            </Field>
            <Field
              label="货币"
              htmlFor="plan-currency"
              hint="3 位 ISO 4217 代码"
              error={errors["plan-currency"]}
            >
              <Input
                id="plan-currency"
                value={form.currency}
                onChange={(e) =>
                  setForm((f) => ({ ...f, currency: e.target.value.toUpperCase() }))
                }
                placeholder="USD"
                maxLength={3}
                autoComplete="off"
                spellCheck={false}
                invalid={Boolean(errors["plan-currency"])}
              />
            </Field>
          </div>

          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm font-medium">定价模型</p>
                <p className="text-xs text-muted-foreground">
                  可叠加多个价格：固定订阅费 + 按量 / 阶梯计费。
                </p>
              </div>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() =>
                  setForm((f) => ({
                    ...f,
                    prices: [...f.prices, newPriceRow("fixed")],
                  }))
                }
              >
                <PlusIcon size={14} aria-hidden="true" />
                添加价格
              </Button>
            </div>
            {errors.prices && (
              <p role="alert" className="text-xs text-danger">
                {errors.prices}
              </p>
            )}
            {form.prices.map((row, index) => (
              <PriceRowEditor
                key={row.id}
                index={index}
                row={row}
                metrics={metrics}
                errors={errors}
                onChange={(next) => setPrice(index, next)}
                onRemove={() =>
                  setForm((f) => ({
                    ...f,
                    prices: f.prices.filter((_, i) => i !== index),
                  }))
                }
                canRemove={form.prices.length > 1}
              />
            ))}
          </div>

          <div className="flex justify-end gap-3">
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" loading={pending}>
              {mode === "create" ? "创建套餐" : "保存修改"}
            </Button>
          </div>
        </form>
      )}
    </Dialog>
  );
}

/* ---------------- 价格项编辑器 ---------------- */
function PriceRowEditor({
  index,
  row,
  metrics,
  errors,
  onChange,
  onRemove,
  canRemove,
}: {
  index: number;
  row: PriceRow;
  metrics: Metric[];
  errors: Record<string, string>;
  onChange: (next: PriceRow) => void;
  onRemove: () => void;
  canRemove: boolean;
}) {
  const metricError = errors[`price-${index}-metric`];
  const amountError = errors[`price-${index}-amount`];
  const tierError = errors[`price-${index}-tiers`];

  return (
    <div className="rounded-lg border border-border bg-surface-2/40 p-3">
      <div className="flex items-center justify-between gap-3">
        <span className="text-xs font-medium text-muted-foreground">
          价格 {index + 1}
        </span>
        {canRemove && (
          <Button type="button" variant="ghost" size="sm" onClick={onRemove}>
            <TrashIcon size={13} aria-hidden="true" />
            移除
          </Button>
        )}
      </div>

      <div className="mt-3 grid gap-3 sm:grid-cols-2">
        <Field
          label="计费模型"
          htmlFor={`price-${index}-model`}
          error={errors[`price-${index}-model`]}
        >
          <Select
            id={`price-${index}-model`}
            value={row.chargeModel}
            onChange={(e) =>
              onChange({ ...row, chargeModel: e.target.value as ChargeModel })
            }
          >
            <option value="fixed">固定价格</option>
            <option value="per_unit">按量计费</option>
            <option value="tiered">阶梯计费</option>
          </Select>
        </Field>

        {row.chargeModel !== "fixed" && (
          <Field
            label="计费指标"
            htmlFor={`price-${index}-metric`}
            hint={
              metrics.length === 0
                ? "当前目录版本暂无指标，暂只能使用固定价格"
                : undefined
            }
            error={metricError}
          >
            <Select
              id={`price-${index}-metric`}
              value={row.metricCode}
              onChange={(e) => onChange({ ...row, metricCode: e.target.value })}
              disabled={metrics.length === 0}
              invalid={Boolean(metricError)}
            >
              <option value="">选择指标</option>
              {metrics.map((m) => (
                <option key={m.id} value={m.code}>
                  {m.name}（{m.code}）
                </option>
              ))}
            </Select>
          </Field>
        )}

        {row.chargeModel !== "tiered" && (
          <Field
            label={row.chargeModel === "fixed" ? "固定价格" : "单价"}
            htmlFor={`price-${index}-amount`}
            hint="以元为单位，例如 29.90"
            error={amountError}
          >
            <Input
              id={`price-${index}-amount`}
              inputMode="decimal"
              value={row.amount}
              onChange={(e) => onChange({ ...row, amount: e.target.value })}
              placeholder="0.00"
              invalid={Boolean(amountError)}
            />
          </Field>
        )}
      </div>

      {row.chargeModel === "tiered" && (
        <div className="mt-3 space-y-2">
          <div className="flex items-center justify-between">
            <span className="text-xs font-medium text-muted-foreground">阶梯区间</span>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() =>
                onChange({ ...row, tiers: [...row.tiers, newTierRow()] })
              }
            >
              <PlusIcon size={13} aria-hidden="true" />
              添加区间
            </Button>
          </div>
          {tierError && (
            <p role="alert" className="text-xs text-danger">
              {tierError}
            </p>
          )}
          <div className="space-y-2">
            {row.tiers.map((tier, ti) => (
              <div
                key={tier.id}
                className="grid grid-cols-2 gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)_auto]"
              >
                <Input
                  aria-label={`区间 ${ti + 1} 起始值`}
                  inputMode="numeric"
                  value={tier.from}
                  onChange={(e) =>
                    onChange({
                      ...row,
                      tiers: row.tiers.map((t, i) =>
                        i === ti ? { ...t, from: e.target.value } : t,
                      ),
                    })
                  }
                  placeholder="从"
                />
                <Input
                  aria-label={`区间 ${ti + 1} 结束值`}
                  inputMode="numeric"
                  value={tier.to}
                  onChange={(e) =>
                    onChange({
                      ...row,
                      tiers: row.tiers.map((t, i) =>
                        i === ti ? { ...t, to: e.target.value } : t,
                      ),
                    })
                  }
                  placeholder="到（最后留空）"
                />
                <Input
                  aria-label={`区间 ${ti + 1} 单价`}
                  inputMode="decimal"
                  value={tier.unitAmount}
                  onChange={(e) =>
                    onChange({
                      ...row,
                      tiers: row.tiers.map((t, i) =>
                        i === ti ? { ...t, unitAmount: e.target.value } : t,
                      ),
                    })
                  }
                  placeholder="单价（元）"
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label={`删除区间 ${ti + 1}`}
                  onClick={() =>
                    onChange({
                      ...row,
                      tiers: row.tiers.filter((_, i) => i !== ti),
                    })
                  }
                >
                  <TrashIcon size={14} aria-hidden="true" />
                </Button>
              </div>
            ))}
          </div>
          <p className="text-xs text-muted-foreground">
            从 0 开始、区间连续，最后一个区间留空表示不设上限。
          </p>
        </div>
      )}
    </div>
  );
}

/* ---------------- 删除套餐 ---------------- */
function DeletePlanDialog({
  open,
  onOpenChange,
  plan,
  providerId,
  env,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  plan: PlanDetail;
  providerId: string;
  env: Env;
}) {
  const router = useRouter();
  const { toast } = useToast();
  const [state, formAction, pending] = useActionState(deletePlanAction, initialState);

  useEffect(() => {
    if (state.ok) {
      toast({ variant: "success", title: "套餐已删除" });
      router.refresh();
      onOpenChange(false);
    }
  }, [state.ok, router, onOpenChange, toast]);

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title={`删除套餐 · ${plan.plan.name}`}
      description={
        <Fragment>
          删除后，{env === "test" ? "测试环境" : "生产环境"}中后续将无法为该套餐新建订阅；
          已存在的订阅仍按版本快照计费，不受影响。此操作写入 draft 版本，未发布前可恢复。
          {state.error && (
            <Alert variant="danger" title="删除失败" className="mt-3">
              {state.error}
            </Alert>
          )}
        </Fragment>
      }
      confirmLabel="删除套餐"
      confirmText={plan.plan.name}
      pending={pending}
      onConfirm={() => {
        const formData = new FormData();
        formData.set("provider_id", providerId);
        formData.set("env", env);
        formData.set("code", plan.plan.code);
        startTransition(() => formAction(formData));
      }}
    />
  );
}
