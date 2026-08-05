"use client";

import { useEffect, useRef, useState } from "react";
import { useActionState } from "react";
import { useRouter } from "next/navigation";
import type {
  CustomDomain,
  NotificationConfig,
  Workspace,
} from "@/lib/api/operator";
import type { Env } from "@/lib/env-shared";
import { formatDate } from "@/lib/format";
import { Button, LinkButton } from "@/components/ui/button";
import { Field, Input, Textarea } from "@/components/ui/field";
import { Dialog, ConfirmDialog } from "@/components/ui/overlay";
import { CopyButton } from "@/components/ui/code-block";
import { EmptyState, ErrorState, Alert, SuccessPanel } from "@/components/ui/feedback";
import { Badge, EnvBadge } from "@/components/ui/badge";
import { DataTable, type DataTableColumn } from "@/components/ui/data-table";
import { Tabs, TabPanel } from "@/components/ui/tabs";
import { useEnv } from "@/components/console/env-provider";
import {
  ArrowRightIcon,
  PlusIcon,
  RefreshIcon,
  SettingsIcon,
  TrashIcon,
} from "@/components/ui/icons";
import {
  deleteDomainAction,
  deleteNotificationAction,
  registerDomainAction,
  revokeDomainAction,
  setNotificationAction,
  updateWorkspaceAction,
  verifyDomainAction,
  type SettingsActionState,
} from "./settings-actions";

const initialState: SettingsActionState = { ok: false };

const DOMAIN_STATUS: Record<string, "success" | "neutral" | "warning"> = {
  verified: "success",
  pending: "warning",
  revoked: "neutral",
};

export function SettingsClient({
  providerId,
  providerName,
  providerSlug,
  env,
  workspace,
  domains,
  notifications,
  loadError,
}: {
  providerId: string | null;
  providerName: string;
  providerSlug: string;
  env: Env;
  workspace: Workspace | null;
  domains: CustomDomain[];
  notifications: NotificationConfig[];
  loadError: string | null;
}) {
  const router = useRouter();
  const { env: activeEnv } = useEnv();
  const prevEnv = useRef(env);
  const [tab, setTab] = useState("basic");
  const [registerOpen, setRegisterOpen] = useState(false);
  const [action, setAction] = useState<{ domain: CustomDomain; mode: "verify" | "revoke" | "delete" } | null>(null);

  useEffect(() => {
    if (prevEnv.current !== activeEnv) {
      prevEnv.current = activeEnv;
      router.refresh();
    }
  }, [activeEnv, router]);

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">设置</h1>
        <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
          管理 workspace 基础信息、自定义认证域名与通知渠道。
          当前环境为 {env === "test" ? "测试环境（沙箱）" : "生产环境（真实配置）"}。
        </p>
      </header>

      {loadError ? (
        <ErrorState
          title="设置加载失败"
          description={loadError}
          action={
            <Button variant="outline" onClick={() => router.refresh()}>
              重试
            </Button>
          }
        />
      ) : !providerId ? (
        <EmptyState
          icon={<SettingsIcon size={20} aria-hidden="true" />}
          title="还没有可管理的 workspace"
          description="先创建并激活 Provider，再回来配置基础信息与域名。"
          action={
            <LinkButton href="/ops" variant="primary" prefetch={false}>
              前往 Provider
              <ArrowRightIcon size={16} aria-hidden="true" />
            </LinkButton>
          }
        />
      ) : (
        <>
          <Tabs
            value={tab}
            onChange={setTab}
            items={[
              { value: "basic", label: "基础" },
              { value: "security", label: "安全" },
              { value: "advanced", label: "高级" },
            ]}
          />
          <TabPanel id="settings" value="basic" selected={tab === "basic"}>
            <BasicSettings
              workspace={workspace}
              providerName={providerName}
              providerSlug={providerSlug}
              env={env}
            />
          </TabPanel>
          <TabPanel id="settings" value="security" selected={tab === "security"}>
            <SecuritySettings
              domains={domains}
              onRegister={() => setRegisterOpen(true)}
              onAction={setAction}
            />
          </TabPanel>
          <TabPanel id="settings" value="advanced" selected={tab === "advanced"}>
            <AdvancedSettings
              providerId={providerId}
              env={env}
              notifications={notifications}
            />
          </TabPanel>
        </>
      )}

      {providerId && (
        <RegisterDomainDialog
          key={registerOpen ? "open" : "closed"}
          open={registerOpen}
          onOpenChange={setRegisterOpen}
          providerId={providerId}
          env={env}
        />
      )}
      {providerId && action && (
        <DomainActionDialog
          key={`${action.domain.id}-${action.mode}`}
          open={!!action}
          onOpenChange={(open) => {
            if (!open) setAction(null);
          }}
          providerId={providerId}
          env={env}
          domain={action.domain}
          mode={action.mode}
        />
      )}
    </div>
  );
}

function BasicSettings({
  workspace,
  providerName,
  providerSlug,
  env,
}: {
  workspace: Workspace | null;
  providerName: string;
  providerSlug: string;
  env: Env;
}) {
  const [state, formAction, pending] = useActionState(updateWorkspaceAction, initialState);

  return (
    <div className="space-y-5">
      <section className="rounded-xl border border-border bg-surface-1 p-5">
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="text-sm font-semibold">基础信息</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              workspace 名称与唯一 slug，slug 用于品牌化登录地址。
            </p>
          </div>
          <EnvBadge env={env} />
        </div>
        {workspace ? (
          <form action={formAction} className="space-y-4">
            <input type="hidden" name="workspace_id" value={workspace.id} />
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="名称" htmlFor="settings-name">
                <Input id="settings-name" name="name" defaultValue={workspace.name} required />
              </Field>
              <Field label="Slug" htmlFor="settings-slug" hint="小写字母、数字与中划线。">
                <Input id="settings-slug" name="slug" defaultValue={workspace.slug} required />
              </Field>
            </div>
            {state.ok && (
              <SuccessPanel title="已保存" description="workspace 基础信息已更新。" />
            )}
            {state.error && <Alert title="保存失败">{state.error}</Alert>}
            <div className="flex justify-end">
              <Button type="submit" loading={pending}>
                保存基础信息
              </Button>
            </div>
          </form>
        ) : (
          <div className="rounded-lg border border-border bg-surface-2 p-4">
            <div className="flex flex-wrap items-center gap-x-6 gap-y-2">
              <div>
                <p className="text-xs text-muted-foreground">名称</p>
                <p className="mt-0.5 font-medium text-foreground">{providerName}</p>
              </div>
              <div>
                <p className="text-xs text-muted-foreground">Slug</p>
                <p className="mt-0.5 font-mono text-sm text-foreground">{providerSlug}</p>
              </div>
            </div>
            <p className="mt-3 text-xs text-muted-foreground">
              当前 provider 未关联 workspace 记录，基础信息只读；注册流程创建的 workspace 可直接编辑。
            </p>
          </div>
        )}
      </section>
    </div>
  );
}

function SecuritySettings({
  domains,
  onRegister,
  onAction,
}: {
  domains: CustomDomain[];
  onRegister: () => void;
  onAction: (action: { domain: CustomDomain; mode: "verify" | "revoke" | "delete" }) => void;
}) {
  const columns: DataTableColumn<CustomDomain>[] = [
    {
      key: "domain",
      header: "域名",
      sortable: true,
      sortValue: (d) => d.domain,
      cell: (d) => <code className="font-mono text-xs text-foreground">{d.domain}</code>,
    },
    {
      key: "status",
      header: "状态",
      sortable: true,
      sortValue: (d) => d.status,
      cell: (d) => (
        <Badge variant={DOMAIN_STATUS[d.status] ?? "neutral"}>{d.status}</Badge>
      ),
    },
    {
      key: "token",
      header: "验证 Token",
      cell: (d) => (
        <code className="font-mono text-[11px] text-muted-foreground break-all">
          {d.verification_token}
        </code>
      ),
    },
    {
      key: "created_at",
      header: "创建时间",
      sortable: true,
      sortValue: (d) => d.created_at ?? "",
      cell: (d) => (
        <span className="text-xs text-muted-foreground tabular-nums">
          {formatDate(d.created_at)}
        </span>
      ),
    },
    {
      key: "actions",
      header: <span className="sr-only">操作</span>,
      className: "text-right",
      cell: (d) => (
        <div className="flex justify-end gap-1">
          {d.status === "pending" && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => onAction({ domain: d, mode: "verify" })}
              aria-label={`验证 ${d.domain}`}
            >
              <RefreshIcon size={14} aria-hidden="true" />
              验证
            </Button>
          )}
          {d.status !== "revoked" && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => onAction({ domain: d, mode: "revoke" })}
              aria-label={`吊销 ${d.domain}`}
            >
              吊销
            </Button>
          )}
          {d.status !== "verified" && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => onAction({ domain: d, mode: "delete" })}
              aria-label={`删除 ${d.domain}`}
            >
              <TrashIcon size={14} aria-hidden="true" />
              删除
            </Button>
          )}
        </div>
      ),
    },
  ];

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold">自定义域名</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            配置品牌化认证域名；注册后添加 TXT 记录完成所有权验证。
          </p>
        </div>
        <Button onClick={onRegister}>
          <PlusIcon size={16} aria-hidden="true" />
          注册域名
        </Button>
      </div>
      {domains.length === 0 ? (
        <EmptyState
          icon={<SettingsIcon size={20} aria-hidden="true" />}
          title="还没有自定义域名"
          description="注册域名后，按提示添加 DNS TXT 记录即可验证。"
          action={
            <Button onClick={onRegister}>
              <PlusIcon size={16} aria-hidden="true" />
              注册第一个域名
            </Button>
          }
        />
      ) : (
        <DataTable
          data={domains}
          columns={columns}
          rowKey={(d) => d.id}
          searchKeys={(d) => [d.domain, d.status, d.verification_token]}
          defaultSort={{ key: "created_at", dir: "desc" }}
          emptyLabel="暂无域名"
        />
      )}
    </div>
  );
}

function AdvancedSettings({
  providerId,
  env,
  notifications,
}: {
  providerId: string;
  env: Env;
  notifications: NotificationConfig[];
}) {
  return (
    <div className="grid gap-5 lg:grid-cols-2">
      <ChannelForm
        providerId={providerId}
        env={env}
        channel="email"
        title="邮件通知"
        config={notifications.find((n) => n.channel === "email")}
        defaultConfig={`{\n  "host": "smtp.example.com",\n  "port": 465,\n  "username": "",\n  "password": ""\n}`}
      />
      <ChannelForm
        providerId={providerId}
        env={env}
        channel="sms"
        title="短信通知"
        config={notifications.find((n) => n.channel === "sms")}
        defaultConfig={`{\n  "account_sid": "",\n  "auth_token": "",\n  "from_number": ""\n}`}
      />
    </div>
  );
}

function ChannelForm({
  providerId,
  env,
  channel,
  title,
  config,
  defaultConfig,
}: {
  providerId: string;
  env: Env;
  channel: "email" | "sms";
  title: string;
  config?: NotificationConfig;
  defaultConfig: string;
}) {
  const [state, formAction, pending] = useActionState(setNotificationAction, initialState);
  const [deleteOpen, setDeleteOpen] = useState(false);

  return (
    <section className="rounded-xl border border-border bg-surface-1 p-5">
      <div className="mb-4 flex items-center justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold">{title}</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {config ? "已配置，可更新渠道凭据。" : "尚未配置。"}
          </p>
        </div>
        {config && (
          <div className="flex items-center gap-2">
            <Badge variant={config.enabled ? "success" : "neutral"}>
              {config.enabled ? "已启用" : "已停用"}
            </Badge>
            <Button variant="ghost" size="sm" onClick={() => setDeleteOpen(true)}>
              删除配置
            </Button>
          </div>
        )}
      </div>
      <form action={formAction} className="space-y-4">
        <input type="hidden" name="provider_id" value={providerId} />
        <input type="hidden" name="env" value={env} />
        <input type="hidden" name="channel" value={channel} />
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="服务商" htmlFor={`${channel}-provider`}>
            <Input
              id={`${channel}-provider`}
              name="provider_type"
              defaultValue={config?.provider_type ?? (channel === "email" ? "smtp" : "twilio")}
              required
            />
          </Field>
          <Field label="发件地址" htmlFor={`${channel}-from`}>
            <Input
              id={`${channel}-from`}
              name="from_address"
              defaultValue={config?.from_address ?? (channel === "email" ? "noreply@example.com" : "")}
              required
              placeholder={channel === "email" ? "noreply@example.com" : "+8613800000000"}
            />
          </Field>
        </div>
        <Field label="配置 JSON" htmlFor={`${channel}-config`} hint="凭据会加密存储，保存后不会明文回显。">
          <Textarea
            id={`${channel}-config`}
            name="config"
            defaultValue={config ? JSON.stringify(config.config, null, 2) : defaultConfig}
            className="min-h-32 font-mono text-xs"
            spellCheck={false}
          />
        </Field>
        <label className="inline-flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            name="enabled"
            defaultChecked={config?.enabled ?? false}
            className="size-4 accent-[var(--brand-600)]"
          />
          启用通知
        </label>
        {state.ok && <SuccessPanel title="已保存" description={`${title}配置已更新。`} />}
        {state.error && <Alert title="保存失败">{state.error}</Alert>}
        <div className="flex justify-end">
          <Button type="submit" loading={pending}>
            保存{title}
          </Button>
        </div>
      </form>
      {config && (
        <DeleteNotificationDialog
          open={deleteOpen}
          onOpenChange={setDeleteOpen}
          providerId={providerId}
          env={env}
          channel={channel}
          title={title}
        />
      )}
    </section>
  );
}

function DeleteNotificationDialog({
  open,
  onOpenChange,
  providerId,
  env,
  channel,
  title,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  providerId: string;
  env: Env;
  channel: "email" | "sms";
  title: string;
}) {
  const router = useRouter();
  const [state, formAction, pending] = useActionState(deleteNotificationAction, initialState);

  function confirm() {
    const fd = new FormData();
    fd.set("provider_id", providerId);
    fd.set("env", env);
    fd.set("channel", channel);
    formAction(fd);
  }

  useEffect(() => {
    if (state.ok) {
      onOpenChange(false);
      router.refresh();
    }
  }, [state.ok, router, onOpenChange]);

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title={`删除${title}配置`}
      description={
        <div className="space-y-2">
          <p>删除后该渠道将停止发送通知，配置与加密凭据一并移除。</p>
          {state.error && <Alert title="删除失败">{state.error}</Alert>}
        </div>
      }
      pending={pending}
      onConfirm={confirm}
      confirmLabel="删除配置"
    />
  );
}

function RegisterDomainDialog({
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
  const [state, formAction, pending] = useActionState(registerDomainAction, initialState);

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title="注册自定义域名"
      description={`域名将注册到${env === "test" ? "测试环境" : "生产环境"}。`}
      size="md"
    >
      {state.ok && state.domain ? (
        <div className="space-y-4">
          <SuccessPanel
            title="域名已注册"
            description="请将下方 TXT 记录添加到 _vlogbin-verify 子域，然后点击验证。"
          >
            <div className="mt-3 space-y-2 rounded-md border border-border bg-surface-2 p-3">
              <code className="block font-mono text-xs text-foreground break-all">
                _vlogbin-verify.{state.domain.domain}
              </code>
              <div className="flex items-center gap-2">
                <code className="min-w-0 flex-1 truncate font-mono text-xs text-foreground">
                  {state.domain.verification_token}
                </code>
                <CopyButton text={state.domain.verification_token} label="复制 Token" />
              </div>
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
          <Field label="域名" htmlFor="domain" hint="例如 auth.example.com，不含协议。">
            <Input id="domain" name="domain" required autoComplete="off" placeholder="auth.example.com" />
          </Field>
          {state.error && <Alert title="注册失败">{state.error}</Alert>}
          <div className="flex justify-end gap-2">
            <Button variant="ghost" type="button" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" loading={pending}>
              注册域名
            </Button>
          </div>
        </form>
      )}
    </Dialog>
  );
}

function DomainActionDialog({
  open,
  onOpenChange,
  providerId,
  env,
  domain,
  mode,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  providerId: string;
  env: Env;
  domain: CustomDomain;
  mode: "verify" | "revoke" | "delete";
}) {
  const router = useRouter();
  const action = mode === "verify" ? verifyDomainAction : mode === "revoke" ? revokeDomainAction : deleteDomainAction;
  const [state, formAction, pending] = useActionState(action, initialState);

  function confirm() {
    const fd = new FormData();
    fd.set("provider_id", providerId);
    fd.set("env", env);
    fd.set("domain_id", domain.id);
    formAction(fd);
  }

  useEffect(() => {
    if (state.ok) {
      onOpenChange(false);
      router.refresh();
    }
  }, [state.ok, router, onOpenChange]);

  const meta = {
    verify: {
      title: "验证自定义域名",
      description: "平台将检查 _vlogbin-verify 子域的 TXT 记录是否包含验证 Token。",
      confirmLabel: "开始验证",
    },
    revoke: {
      title: "吊销自定义域名",
      description: "吊销后该域名不再路由到当前环境，记录保留用于审计。",
      confirmLabel: "吊销域名",
    },
    delete: {
      title: "删除自定义域名",
      description: "删除后记录将永久移除。已认证域名需先吊销。",
      confirmLabel: "删除域名",
    },
  }[mode];

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title={meta.title}
      description={
        <div className="space-y-2">
          <p>{meta.description}</p>
          <code className="block font-mono text-xs text-foreground break-all">{domain.domain}</code>
          {state.error && <Alert title="操作失败">{state.error}</Alert>}
        </div>
      }
      confirmText={mode === "delete" ? domain.domain : undefined}
      pending={pending}
      onConfirm={confirm}
      confirmLabel={meta.confirmLabel}
    />
  );
}
