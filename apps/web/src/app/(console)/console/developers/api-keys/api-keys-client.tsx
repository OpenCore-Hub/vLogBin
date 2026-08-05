"use client";

import {
  useActionState,
  useEffect,
  useRef,
  useState,
} from "react";
import { useRouter } from "next/navigation";
import type { Credential } from "@/lib/api/operator";
import type { Env } from "@/lib/env-shared";
import { formatDate } from "@/lib/format";
import { Button, LinkButton } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/field";
import { Dialog, ConfirmDialog, DropdownMenu } from "@/components/ui/overlay";
import { CopyButton } from "@/components/ui/code-block";
import { EmptyState, ErrorState, Alert, SuccessPanel } from "@/components/ui/feedback";
import { Badge, EnvBadge } from "@/components/ui/badge";
import { DataTable, type DataTableColumn } from "@/components/ui/data-table";
import { useEnv } from "@/components/console/env-provider";
import {
  ArrowRightIcon,
  KeyIcon,
  KebabIcon,
  PlusIcon,
  RefreshIcon,
  TrashIcon,
} from "@/components/ui/icons";
import { useActionFeedback } from "@/hooks/use-action-feedback";
import {
  createCredentialAction,
  revokeCredentialAction,
  rotateCredentialAction,
  type ApiKeyActionState,
} from "./api-keys-actions";

const initialState: ApiKeyActionState = { ok: false };

const SCOPE_OPTIONS = [
  { value: "read", label: "读", description: "读取资源" },
  { value: "write", label: "写", description: "创建与更新资源" },
  { value: "credentials:manage", label: "凭证管理", description: "签发与吊销密钥" },
  { value: "audit:read", label: "审计读", description: "读取审计日志" },
  { value: "support:approve", label: "支持审批", description: "审批临时访问" },
  { value: "scim:manage", label: "SCIM", description: "管理目录用户与组" },
];

const EXPIRY_OPTIONS = [
  { value: "", label: "永不过期" },
  { value: "30", label: "30 天后" },
  { value: "90", label: "90 天后" },
  { value: "365", label: "365 天后" },
];

export function ApiKeysClient({
  providerId,
  env,
  credentials,
  loadError,
}: {
  providerId: string | null;
  env: Env;
  credentials: Credential[];
  loadError: string | null;
}) {
  const router = useRouter();
  const { env: activeEnv } = useEnv();
  const prevEnv = useRef(env);
  const [createOpen, setCreateOpen] = useState(false);
  const [createNonce, setCreateNonce] = useState(0);
  const [rotating, setRotating] = useState<Credential | null>(null);
  const [deleting, setDeleting] = useState<Credential | null>(null);

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
          <h1 className="text-2xl font-semibold tracking-tight">API Keys</h1>
          <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
            管理接入 vLogBin API 的密钥。密钥创建或轮换后只显示一次，请立即保存。
            当前环境为 {env === "test" ? "测试环境（沙箱）" : "生产环境（真实数据）"}。
          </p>
        </div>
        {providerId && (
          <Button onClick={openCreate}>
            <PlusIcon size={16} aria-hidden="true" />
            创建密钥
          </Button>
        )}
      </header>

      {loadError ? (
        <ErrorState
          title="密钥列表加载失败"
          description={loadError}
          action={
            <Button variant="outline" onClick={() => router.refresh()}>
              重试
            </Button>
          }
        />
      ) : !providerId ? (
        <EmptyState
          icon={<KeyIcon size={20} aria-hidden="true" />}
          title="还没有可管理的 workspace"
          description="先创建并激活 Provider，获得测试环境后即可签发 API 密钥。"
          action={
            <LinkButton href="/ops" variant="primary" prefetch={false}>
              前往 Provider
              <ArrowRightIcon size={16} aria-hidden="true" />
            </LinkButton>
          }
        />
      ) : credentials.length === 0 ? (
        <EmptyState
          icon={<KeyIcon size={20} aria-hidden="true" />}
          title="还没有 API 密钥"
          description={`在${env === "test" ? "测试环境" : "生产环境"}签发第一枚密钥，最小权限原则建议按角色拆分。`}
          action={
            <Button onClick={openCreate}>
              <PlusIcon size={16} aria-hidden="true" />
              创建第一枚密钥
            </Button>
          }
        />
      ) : (
        <KeyTable
          credentials={credentials}
          env={env}
          onRotate={setRotating}
          onRevoke={setDeleting}
        />
      )}

      {providerId && (
        <CreateKeyDialog
          key={`create-${createNonce}`}
          open={createOpen}
          onOpenChange={setCreateOpen}
          providerId={providerId}
          env={env}
        />
      )}
      {providerId && rotating && (
        <RotateKeyDialog
          open={!!rotating}
          onOpenChange={(open) => {
            if (!open) setRotating(null);
          }}
          providerId={providerId}
          env={env}
          credential={rotating}
        />
      )}
      {providerId && deleting && (
        <RevokeKeyDialog
          open={!!deleting}
          onOpenChange={(open) => {
            if (!open) setDeleting(null);
          }}
          providerId={providerId}
          credential={deleting}
        />
      )}
    </div>
  );
}

function KeyTable({
  credentials,
  env,
  onRotate,
  onRevoke,
}: {
  credentials: Credential[];
  env: Env;
  onRotate: (credential: Credential) => void;
  onRevoke: (credential: Credential) => void;
}) {
  const columns: DataTableColumn<Credential>[] = [
    {
      key: "name",
      header: "名称",
      sortable: true,
      sortValue: (c) => c.name,
      cell: (c) => <span className="font-medium text-foreground">{c.name}</span>,
    },
    {
      key: "prefix",
      header: "前缀",
      cell: (c) => (
        <code className="font-mono text-xs text-muted-foreground tabular-nums">
          {c.key_prefix}…
        </code>
      ),
    },
    {
      key: "scopes",
      header: "权限",
      cell: (c) => (
        <div className="flex max-w-md flex-wrap gap-1">
          {c.scopes.map((scope) => (
            <code
              key={scope}
              className="rounded-md bg-surface-2 px-1.5 py-0.5 font-mono text-[11px] text-foreground"
            >
              {scope}
            </code>
          ))}
        </div>
      ),
    },
    {
      key: "status",
      header: "状态",
      sortable: true,
      sortValue: (c) =>
        c.revoked_at
          ? 2
          : c.expires_at && new Date(c.expires_at) < new Date()
            ? 1
            : 0,
      cell: (c) => {
        if (c.revoked_at) return <Badge variant="neutral">已吊销</Badge>;
        if (c.expires_at && new Date(c.expires_at) < new Date()) {
          return <Badge variant="warning">已过期</Badge>;
        }
        return <Badge variant="success">有效</Badge>;
      },
    },
    {
      key: "environment",
      header: "环境",
      cell: () => <EnvBadge env={env} />,
    },
    {
      key: "last_used_at",
      header: "最近使用",
      sortable: true,
      sortValue: (c) => c.last_used_at ?? "",
      cell: (c) => (
        <span className="text-xs text-muted-foreground tabular-nums">
          {c.last_used_at ? formatDate(c.last_used_at) : "—"}
        </span>
      ),
    },
    {
      key: "created_at",
      header: "创建时间",
      sortable: true,
      sortValue: (c) => c.created_at ?? "",
      cell: (c) => (
        <span className="text-xs text-muted-foreground tabular-nums">
          {formatDate(c.created_at)}
        </span>
      ),
    },
    {
      key: "actions",
      header: <span className="sr-only">操作</span>,
      className: "text-right",
      cell: (c) =>
        c.revoked_at ? (
          <span className="text-xs text-muted-foreground">已吊销</span>
        ) : (
          <DropdownMenu
            align="end"
            triggerLabel={`${c.name} 操作`}
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
                    <RefreshIcon size={14} aria-hidden="true" />
                    轮换密钥
                  </span>
                ),
                onSelect: () => onRotate(c),
              },
              { type: "separator" },
              {
                type: "item",
                danger: true,
                label: (
                  <span className="inline-flex items-center gap-2">
                    <TrashIcon size={14} aria-hidden="true" />
                    吊销密钥
                  </span>
                ),
                onSelect: () => onRevoke(c),
              },
            ]}
          />
        ),
    },
  ];

  return (
    <DataTable
      data={credentials}
      columns={columns}
      rowKey={(c) => c.id}
      searchKeys={(c) => [c.name, c.key_prefix, ...c.scopes]}
      defaultSort={{ key: "created_at", dir: "desc" }}
      emptyLabel="暂无密钥"
    />
  );
}

function CreateKeyDialog({
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
  const [state, formAction, pending] = useActionState(createCredentialAction, initialState);

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title="创建 API 密钥"
      description={`密钥将签发到${env === "test" ? "测试环境" : "生产环境"}，创建后只显示一次。`}
      size="md"
    >
      {state.ok && state.apiKey ? (
        <div className="space-y-4">
          <SuccessPanel
            title="密钥创建成功"
            description="请立即复制并保存；关闭后平台不会再次显示明文。"
          >
            <div className="mt-3 flex items-center gap-2 rounded-md border border-border bg-surface-2 p-3">
              <code className="min-w-0 flex-1 truncate font-mono text-xs text-foreground">
                {state.apiKey}
              </code>
              <CopyButton text={state.apiKey} label="复制密钥" />
            </div>
          </SuccessPanel>
          <div className="flex justify-end">
            <Button onClick={() => onOpenChange(false)}>完成</Button>
          </div>
        </div>
      ) : (
        <form action={formAction} className="space-y-4">
          <input type="hidden" name="provider_id" value={providerId} />
          <input type="hidden" name="env" value={env} />
          <Field label="密钥名称" htmlFor="name" hint="用于识别用途，例如 ci、staging、billing-service。">
            <Input id="name" name="name" autoComplete="off" required placeholder="billing-service" />
          </Field>
          <fieldset>
            <legend className="mb-1.5 text-sm font-medium text-foreground">权限</legend>
            <div className="grid gap-2 rounded-lg border border-border bg-surface-1 p-3 sm:grid-cols-2">
              {SCOPE_OPTIONS.map((scope) => (
                <label key={scope.value} className="flex items-start gap-2.5 text-sm">
                  <input
                    type="checkbox"
                    name="scopes"
                    value={scope.value}
                    className="mt-0.5 size-4 accent-[var(--brand-600)]"
                  />
                  <span>
                    <span className="block font-medium text-foreground">{scope.label}</span>
                    <span className="block text-xs text-muted-foreground">{scope.description}</span>
                  </span>
                </label>
              ))}
            </div>
          </fieldset>
          <Field label="过期时间" htmlFor="expires">
            <Select id="expires" name="expires" defaultValue="">
              {EXPIRY_OPTIONS.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </Select>
          </Field>
          {state.error && <Alert title="创建失败">{state.error}</Alert>}
          <div className="flex justify-end gap-2">
            <Button variant="ghost" type="button" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" loading={pending}>
              创建密钥
            </Button>
          </div>
        </form>
      )}
    </Dialog>
  );
}

function RotateKeyDialog({
  open,
  onOpenChange,
  providerId,
  env,
  credential,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  providerId: string;
  env: Env;
  credential: Credential;
}) {
  const [state, formAction, pending] = useActionState(rotateCredentialAction, initialState);

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title="轮换 API 密钥"
      description="新密钥保留相同名称与权限，旧密钥立即失效。"
      size="md"
    >
      {state.ok && state.apiKey ? (
        <div className="space-y-4">
          <SuccessPanel
            title="密钥已轮换"
            description={`${credential.name} 的旧密钥已吊销，新密钥只显示一次。`}
          >
            <div className="mt-3 flex items-center gap-2 rounded-md border border-border bg-surface-2 p-3">
              <code className="min-w-0 flex-1 truncate font-mono text-xs text-foreground">
                {state.apiKey}
              </code>
              <CopyButton text={state.apiKey} label="复制密钥" />
            </div>
          </SuccessPanel>
          <div className="flex justify-end">
            <Button onClick={() => onOpenChange(false)}>完成</Button>
          </div>
        </div>
      ) : (
        <form action={formAction} className="space-y-4">
          <input type="hidden" name="provider_id" value={providerId} />
          <input type="hidden" name="env" value={env} />
          <input type="hidden" name="credential_id" value={credential.id} />
          <p className="rounded-lg border border-warning/25 bg-warning-soft px-3 py-2.5 text-sm text-warning">
            {credential.name}（{credential.key_prefix}…）
          </p>
          {state.error && <Alert title="轮换失败">{state.error}</Alert>}
          <div className="flex justify-end gap-2">
            <Button variant="ghost" type="button" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" loading={pending}>
              确认轮换
            </Button>
          </div>
        </form>
      )}
    </Dialog>
  );
}

function RevokeKeyDialog({
  open,
  onOpenChange,
  providerId,
  credential,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  providerId: string;
  credential: Credential;
}) {
  const router = useRouter();
  const { state, formAction, pending } = useActionFeedback<ApiKeyActionState>({
    action: revokeCredentialAction,
    initialState,
    onSuccess: () => {
      onOpenChange(false);
      router.refresh();
    },
    successTitle: "API 密钥已吊销",
  });

  function confirm() {
    const fd = new FormData();
    fd.set("provider_id", providerId);
    fd.set("credential_id", credential.id);
    formAction(fd);
  }

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title="吊销 API 密钥"
      description={
        <div className="space-y-2">
          <p>
            吊销后 <span className="font-medium text-foreground">{credential.name}</span>（
            <span className="font-mono">{credential.key_prefix}…</span>）将立即失效。
          </p>
          {state.error && <Alert title="吊销失败">{state.error}</Alert>}
        </div>
      }
      confirmText={credential.name}
      pending={pending}
      onConfirm={confirm}
      confirmLabel="吊销密钥"
    />
  );
}
