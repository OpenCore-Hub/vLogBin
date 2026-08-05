"use client";

import {
  keepPreviousData,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import type { EntitlementGrant, PlanDetail } from "@/lib/api/operator";
import type { Env } from "@/lib/env-shared";
import { consoleQueryKeys, QUERY_STALE_TIME } from "@/hooks/query-keys";
import { useActionFeedback } from "@/hooks/use-action-feedback";
import { useEnv } from "@/components/console/env-provider";
import { Button, LinkButton } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/field";
import { Dialog, ConfirmDialog } from "@/components/ui/overlay";
import {
  Alert,
  EmptyState,
  ErrorState,
  Spinner,
} from "@/components/ui/feedback";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import {
  ArrowRightIcon,
  EditIcon,
  PlusIcon,
  ShieldIcon,
  TrashIcon,
} from "@/components/ui/icons";
import {
  deleteEntitlementAction,
  listEntitlementsAction,
  setEntitlementAction,
  type PoliciesActionState,
} from "./policies-actions";

const initialState: PoliciesActionState = { ok: false };

const VALUE_TYPE_VARIANT: Record<
  string,
  "success" | "info" | "warning" | "neutral"
> = {
  boolean: "info",
  numeric: "success",
  period: "warning",
};

export function PoliciesClient({
  providerId,
  env,
  plans,
  initialPlanCode,
  initialEntitlements,
  loadError,
}: {
  providerId: string | null;
  env: Env;
  plans: PlanDetail[];
  initialPlanCode: string | null;
  initialEntitlements: EntitlementGrant[];
  loadError: string | null;
}) {
  const router = useRouter();
  const { env: activeEnv } = useEnv();
  const prevEnv = useRef(env);
  const queryClient = useQueryClient();
  const [selectedPlanCode, setSelectedPlanCode] = useState(
    initialPlanCode ?? plans[0]?.plan.code ?? "",
  );
  const [editor, setEditor] = useState<{
    planCode: string;
    grant?: EntitlementGrant;
  } | null>(null);
  const [deleting, setDeleting] = useState<{
    planCode: string;
    grant: EntitlementGrant;
  } | null>(null);

  useEffect(() => {
    if (prevEnv.current !== activeEnv) {
      prevEnv.current = activeEnv;
      router.refresh();
    }
  }, [activeEnv, router]);

  const selectedPlan =
    plans.find((p) => p.plan.code === selectedPlanCode) ?? plans[0] ?? null;
  const activePlanCode = selectedPlan?.plan.code ?? "";
  const policyKey = consoleQueryKeys.policies(providerId, env, activePlanCode);

  const entitlementsQuery = useQuery({
    queryKey: policyKey,
    queryFn: async () => {
      if (!providerId || !selectedPlan) return [];
      const result = await listEntitlementsAction({
        providerId,
        env,
        planCode: selectedPlan.plan.code,
      });
      if (!result.ok) {
        throw new Error(result.error ?? "权益列表加载失败");
      }
      return result.grants;
    },
    enabled: Boolean(providerId && selectedPlan),
    placeholderData: keepPreviousData,
    staleTime: QUERY_STALE_TIME.standard,
    initialData:
      selectedPlan && activePlanCode === initialPlanCode
        ? initialEntitlements
        : undefined,
  });

  const grants = entitlementsQuery.data ?? [];
  const queryError = entitlementsQuery.isError
    ? entitlementsQuery.error instanceof Error
      ? entitlementsQuery.error.message
      : "权益列表加载失败"
    : null;

  function invalidatePolicies() {
    void queryClient.invalidateQueries({ queryKey: policyKey });
  }

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Policies</h1>
          <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
            配置套餐级权益授权：谁能用什么功能，按 plan 独立管理。
          </p>
        </div>
        {providerId && selectedPlan && (
          <Button
            onClick={() =>
              setEditor({ planCode: selectedPlan.plan.code })
            }
          >
            <PlusIcon size={16} aria-hidden="true" />
            添加权益
          </Button>
        )}
      </header>

      {loadError ? (
        <ErrorState title="策略加载失败" description={loadError} />
      ) : !providerId ? (
        <EmptyState
          icon={<ShieldIcon size={20} aria-hidden="true" />}
          title="还没有可管理的 workspace"
          description="先创建并激活 Provider，再为套餐配置权益策略。"
          action={
            <LinkButton href="/ops" variant="primary" prefetch={false}>
              前往 Provider
              <ArrowRightIcon size={16} aria-hidden="true" />
            </LinkButton>
          }
        />
      ) : plans.length === 0 ? (
        <EmptyState
          icon={<ShieldIcon size={20} aria-hidden="true" />}
          title="还没有套餐"
          description="先创建套餐，再为每个套餐配置功能权益。"
          action={
            <LinkButton
              href="/console/billing/plans"
              variant="primary"
              prefetch={false}
            >
              前往套餐
              <ArrowRightIcon size={16} aria-hidden="true" />
            </LinkButton>
          }
        />
      ) : (
        <Card className="overflow-hidden">
          <div className="flex flex-wrap items-center gap-3 border-b border-border px-4 py-3">
            <Select
              aria-label="选择套餐"
              value={selectedPlan?.plan.code ?? ""}
              onChange={(e) => {
                setSelectedPlanCode(e.target.value);
                setEditor(null);
                setDeleting(null);
              }}
              className="h-9 w-64 text-sm"
            >
              {plans.map((p) => (
                <option key={p.plan.code} value={p.plan.code}>
                  {p.plan.name} · {p.plan.code}
                </option>
              ))}
            </Select>
            <span className="text-xs text-muted-foreground">
              {activePlanCode} 的权益授权
            </span>
            {entitlementsQuery.isPending && (
              <Spinner size={14} label="加载权益列表" />
            )}
          </div>

          {queryError && (
            <div className="px-4 pt-4">
              <Alert title="加载失败">{queryError}</Alert>
            </div>
          )}

          {grants.length === 0 ? (
            <EmptyState
              icon={<ShieldIcon size={20} aria-hidden="true" />}
              title="暂无权益"
              description="为该套餐添加第一条权益，例如 max_users、feature_export。"
              className="border-0 shadow-none"
            />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead className="bg-surface-2/70 text-left text-xs font-medium text-muted-foreground">
                  <tr>
                    <th className="px-4 py-3 font-medium">权益</th>
                    <th className="px-4 py-3 font-medium">类型</th>
                    <th className="px-4 py-3 font-medium">值</th>
                    <th className="px-4 py-3 text-right font-medium">操作</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border">
                  {grants.map((grant) => (
                    <tr key={grant.id} className="transition-colors hover:bg-surface-2/60">
                      <td className="px-4 py-3">
                        <code className="font-mono text-xs font-medium text-foreground">
                          {grant.key}
                        </code>
                      </td>
                      <td className="px-4 py-3">
                        <Badge variant={VALUE_TYPE_VARIANT[grant.value_type] ?? "neutral"}>
                          {grant.value_type}
                        </Badge>
                      </td>
                      <td className="px-4 py-3">
                        <code className="font-mono text-xs text-muted-foreground">
                          {JSON.stringify(grant.value)}
                        </code>
                      </td>
                      <td className="px-4 py-3 text-right">
                        <div className="inline-flex items-center gap-1">
                          <Button
                            variant="ghost"
                            size="sm"
                            aria-label={`编辑 ${grant.key}`}
                            onClick={() =>
                              setEditor({
                                planCode: selectedPlan?.plan.code ?? "",
                                grant,
                              })
                            }
                          >
                            <EditIcon size={14} aria-hidden="true" />
                            编辑
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            aria-label={`删除 ${grant.key}`}
                            onClick={() =>
                              setDeleting({
                                planCode: selectedPlan?.plan.code ?? "",
                                grant,
                              })
                            }
                          >
                            <TrashIcon size={14} aria-hidden="true" />
                            删除
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>
      )}

      {editor && (
        <EntitlementDialog
          key={`${editor.planCode}-${editor.grant?.key ?? "new"}`}
          open
          providerId={providerId!}
          env={env}
          planCode={editor.planCode}
          grant={editor.grant}
          onOpenChange={(open) => {
            if (!open) setEditor(null);
          }}
          onSaved={invalidatePolicies}
        />
      )}

      {deleting && (
        <DeleteEntitlementDialog
          key={`delete-${deleting.planCode}-${deleting.grant.key}`}
          open
          providerId={providerId!}
          env={env}
          planCode={deleting.planCode}
          grant={deleting.grant}
          onOpenChange={(open) => {
            if (!open) setDeleting(null);
          }}
          onDeleted={invalidatePolicies}
        />
      )}
    </div>
  );
}

function EntitlementDialog({
  open,
  providerId,
  env,
  planCode,
  grant,
  onOpenChange,
  onSaved,
}: {
  open: boolean;
  providerId: string;
  env: Env;
  planCode: string;
  grant?: EntitlementGrant;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
}) {
  const [key, setKey] = useState(grant?.key ?? "");
  const [valueType, setValueType] = useState(grant?.value_type ?? "boolean");
  const [valueJson, setValueJson] = useState(() =>
    grant ? JSON.stringify(grant.value ?? "") : "true",
  );
  const { state, formAction, pending } = useActionFeedback<PoliciesActionState>({
    action: setEntitlementAction,
    initialState,
    onSuccess: () => {
      onOpenChange(false);
      onSaved();
    },
    successTitle: grant ? "权益已更新" : "权益已创建",
  });

  function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const fd = new FormData();
    fd.set("provider_id", providerId);
    fd.set("env", env);
    fd.set("plan_code", planCode);
    fd.set("key", key.trim());
    fd.set("value_type", valueType);
    if (valueType === "period" && !valueJson.trim().startsWith('"')) {
      fd.set("value", JSON.stringify(valueJson.trim()));
    } else {
      fd.set("value", valueJson);
    }
    formAction(fd);
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={grant ? "编辑权益" : "添加权益"}
      description={`作用于 ${planCode} 的当前 draft 目录版本。`}
      size="md"
    >
      <form onSubmit={submit} className="space-y-4">
        <Field label="权益 key" htmlFor="key" hint="例如 max_users、feature_export，创建后不可修改。">
          <Input
            id="key"
            value={key}
            disabled={Boolean(grant)}
            onChange={(e) => setKey(e.target.value)}
            autoComplete="off"
            placeholder="feature_export"
          />
        </Field>
        <Field label="值类型" htmlFor="value_type">
          <Select
            id="value_type"
            value={valueType}
            onChange={(e) => {
              const next = e.target.value;
              setValueType(next);
              setValueJson(
                next === "boolean" ? "true" : next === "numeric" ? "10" : '"30d"',
              );
            }}
          >
            <option value="boolean">boolean</option>
            <option value="numeric">numeric</option>
            <option value="period">period</option>
          </Select>
        </Field>
        <Field
          label="值"
          htmlFor="value"
          hint='按 JSON 输入：true、10 或 "30d"。'
        >
          {valueType === "boolean" ? (
            <Select
              id="value"
              value={valueJson}
              onChange={(e) => setValueJson(e.target.value)}
            >
              <option value="true">true</option>
              <option value="false">false</option>
            </Select>
          ) : valueType === "numeric" ? (
            <Input
              id="value"
              type="number"
              step="any"
              value={valueJson}
              onChange={(e) => setValueJson(e.target.value)}
              placeholder="10"
            />
          ) : (
            <Input
              id="value"
              value={valueJson}
              onChange={(e) => setValueJson(e.target.value)}
              placeholder='"30d"'
            />
          )}
        </Field>
        {state.error && <Alert title="保存失败">{state.error}</Alert>}
        <div className="flex justify-end gap-2">
          <Button variant="ghost" type="button" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button type="submit" loading={pending}>
            {grant ? "保存修改" : "添加权益"}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}

function DeleteEntitlementDialog({
  open,
  providerId,
  env,
  planCode,
  grant,
  onOpenChange,
  onDeleted,
}: {
  open: boolean;
  providerId: string;
  env: Env;
  planCode: string;
  grant: EntitlementGrant;
  onOpenChange: (open: boolean) => void;
  onDeleted: () => void;
}) {
  const { state, formAction, pending } = useActionFeedback<PoliciesActionState>({
    action: deleteEntitlementAction,
    initialState,
    onSuccess: () => {
      onOpenChange(false);
      onDeleted();
    },
    successTitle: "权益已删除",
  });

  function confirm() {
    const fd = new FormData();
    fd.set("provider_id", providerId);
    fd.set("env", env);
    fd.set("plan_code", planCode);
    fd.set("key", grant.key);
    formAction(fd);
  }

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title="删除权益"
      description={
        <div className="space-y-2">
          <p>
            将删除 <code className="font-mono text-xs">{grant.key}</code> 权益。
            输入权益 key 确认。
          </p>
          {state.error && <Alert title="删除失败">{state.error}</Alert>}
        </div>
      }
      confirmText={grant.key}
      pending={pending}
      onConfirm={confirm}
      confirmLabel="删除权益"
    />
  );
}
