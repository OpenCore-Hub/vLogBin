"use client";

import { useActionState, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import type { Customer } from "@/lib/api/operator";
import { createCustomerSchema } from "@/lib/api/schemas";
import type { Env } from "@/lib/env-shared";
import { formatDate } from "@/lib/format";
import { Button, LinkButton } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/field";
import { Dialog } from "@/components/ui/overlay";
import { EmptyState, ErrorState, Alert, SuccessPanel } from "@/components/ui/feedback";
import { Badge, EnvBadge } from "@/components/ui/badge";
import { useEnv } from "@/components/console/env-provider";
import {
  ArrowRightIcon,
  PlusIcon,
  UsersIcon,
} from "@/components/ui/icons";
import {
  createCustomerAction,
  type CustomerActionState,
} from "./customers-actions";

const initialState: CustomerActionState = { ok: false };

export function CustomersClient({
  providerId,
  env,
  customers,
  loadError,
}: {
  providerId: string | null;
  env: Env;
  customers: Customer[];
  loadError: string | null;
}) {
  const router = useRouter();
  const { env: activeEnv } = useEnv();
  const prevEnv = useRef(env);
  const [createOpen, setCreateOpen] = useState(false);
  const [createNonce, setCreateNonce] = useState(0);

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
          <h1 className="text-2xl font-semibold tracking-tight">客户</h1>
          <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
            管理使用你产品的终端客户。客户创建后可查看订阅、用量与账单。
            当前环境为 {env === "test" ? "测试环境（沙箱）" : "生产环境（真实客户生效）"}。
          </p>
        </div>
        {providerId && (
          <Button onClick={openCreate}>
            <PlusIcon size={16} aria-hidden="true" />
            创建客户
          </Button>
        )}
      </header>

      {loadError ? (
        <ErrorState
          title="客户列表加载失败"
          description={loadError}
          action={
            <Button variant="outline" onClick={() => router.refresh()}>
              重试
            </Button>
          }
        />
      ) : !providerId ? (
        <EmptyState
          icon={<UsersIcon size={20} aria-hidden="true" />}
          title="还没有可管理的 workspace"
          description="先创建并激活 Provider，获得测试环境后即可创建第一个客户。"
          action={
            <LinkButton href="/ops" variant="primary" prefetch={false}>
              前往 Provider
              <ArrowRightIcon size={16} aria-hidden="true" />
            </LinkButton>
          }
        />
      ) : customers.length === 0 ? (
        <EmptyState
          icon={<UsersIcon size={20} aria-hidden="true" />}
          title="还没有客户"
          description={`在${env === "test" ? "测试环境" : "生产环境"}创建第一个客户，之后即可订阅套餐并产生用量与账单。`}
          action={
            <Button onClick={openCreate}>
              <PlusIcon size={16} aria-hidden="true" />
              创建第一个客户
            </Button>
          }
        />
      ) : (
        <div className="overflow-x-auto rounded-xl border border-border bg-surface-1">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border bg-surface-2 text-left text-xs font-medium text-muted-foreground">
                <th className="px-4 py-3 font-medium">客户</th>
                <th className="px-4 py-3 font-medium">类型</th>
                <th className="px-4 py-3 font-medium">环境</th>
                <th className="px-4 py-3 font-medium">创建时间</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {customers.map((c) => (
                <tr key={c.id} className="transition-colors hover:bg-surface-2/60">
                  <td className="px-4 py-3">
                    <Link
                      href={`/console/billing/customers/${encodeURIComponent(c.external_id)}`}
                      prefetch={false}
                      className="group flex items-center gap-2"
                    >
                      <span className="font-medium text-foreground group-hover:text-brand-700 dark:group-hover:text-brand-400">
                        {c.display_name}
                      </span>
                      <span className="font-mono text-xs text-muted-foreground">
                        {c.external_id}
                      </span>
                      <ArrowRightIcon
                        size={14}
                        className="text-muted-foreground transition-transform group-hover:translate-x-0.5"
                        aria-hidden="true"
                      />
                    </Link>
                  </td>
                  <td className="px-4 py-3">
                    <Badge variant={c.account_type === "business" ? "brand" : "info"}>
                      {c.account_type === "business" ? "企业" : "个人"}
                    </Badge>
                  </td>
                  <td className="px-4 py-3">
                    <EnvBadge env={env} />
                  </td>
                  <td className="px-4 py-3 text-xs text-muted-foreground tabular-nums">
                    {formatDate(c.created_at)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {providerId && (
        <CreateCustomerDialog
          key={`create-${createNonce}`}
          open={createOpen}
          onOpenChange={setCreateOpen}
          providerId={providerId}
          env={env}
        />
      )}
    </div>
  );
}

/* ---------------- 创建客户 ---------------- */
function CreateCustomerDialog({
  open,
  onOpenChange,
  providerId,
  env,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  providerId: string;
  env: Env;
}) {
  const router = useRouter();
  const [state, formAction, pending] = useActionState(createCustomerAction, initialState);
  const [errors, setErrors] = useState<Record<string, string>>({});

  useEffect(() => {
    if (state.ok) router.refresh();
  }, [state.ok, router]);

  function submit(formData: FormData) {
    const parsed = createCustomerSchema.safeParse({
      external_id: formData.get("external_id"),
      account_type: formData.get("account_type"),
      display_name: formData.get("display_name"),
    });
    if (!parsed.success) {
      const next: Record<string, string> = {};
      for (const issue of parsed.error.issues) {
        const key = issue.path[0] ? String(issue.path[0]) : "form";
        if (!next[key]) next[key] = issue.message;
      }
      setErrors(next);
      const first = Object.keys(next)[0];
      if (first) {
        requestAnimationFrame(() => document.getElementById(first)?.focus());
      }
      return;
    }
    setErrors({});
    formData.set("provider_id", providerId);
    formData.set("env", env);
    formAction(formData);
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title="创建客户"
      description={`客户将创建在${env === "test" ? "测试环境" : "生产环境"}，external_id 在同一环境内唯一。`}
      size="md"
    >
      {state.ok && state.customer ? (
        <div className="space-y-4">
          <SuccessPanel
            title="客户创建成功"
            description={`${state.customer.display_name} 已加入 ${env === "test" ? "测试" : "生产"}环境。`}
          >
            <div className="mt-3 rounded-md border border-border bg-surface-2 p-3">
              <span className="block font-mono text-xs text-foreground">
                {state.customer.external_id}
              </span>
              <span className="mt-1 block text-xs text-muted-foreground">
                {state.customer.account_type === "business" ? "企业客户" : "个人客户"}
              </span>
            </div>
          </SuccessPanel>
          <div className="flex flex-wrap items-center justify-end gap-3">
            <Button variant="ghost" onClick={() => onOpenChange(false)}>
              返回客户列表
            </Button>
            <LinkButton
              href={`/console/billing/customers/${encodeURIComponent(state.customer.external_id)}`}
              variant="primary"
              prefetch={false}
              onClick={() => onOpenChange(false)}
            >
              查看客户详情
              <ArrowRightIcon size={16} aria-hidden="true" />
            </LinkButton>
          </div>
        </div>
      ) : (
        <form action={submit} className="space-y-4" noValidate>
          {state.error && (
            <Alert variant="danger" title="创建失败">
              {state.error}
            </Alert>
          )}
          <Field
            label="客户名称"
            htmlFor="display_name"
            error={errors.display_name}
          >
            <Input
              id="display_name"
              name="display_name"
              placeholder="例如：Acme Corp"
              autoFocus
              autoComplete="off"
              invalid={Boolean(errors.display_name)}
            />
          </Field>
          <Field
            label="客户外部 ID"
            htmlFor="external_id"
            hint="你的系统里对客户的唯一标识，例如客户编号"
            error={errors.external_id}
          >
            <Input
              id="external_id"
              name="external_id"
              placeholder="cust_12345"
              autoComplete="off"
              spellCheck={false}
              invalid={Boolean(errors.external_id)}
            />
          </Field>
          <Field label="客户类型" htmlFor="account_type" error={errors.account_type}>
            <Select
              id="account_type"
              name="account_type"
              defaultValue="business"
              invalid={Boolean(errors.account_type)}
            >
              <option value="business">企业</option>
              <option value="individual">个人</option>
            </Select>
          </Field>
          <div className="flex justify-end gap-3">
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" loading={pending}>
              <PlusIcon size={16} aria-hidden="true" />
              创建客户
            </Button>
          </div>
        </form>
      )}
    </Dialog>
  );
}
