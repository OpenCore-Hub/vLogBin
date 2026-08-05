"use client";

import { startTransition, useActionState, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import type { HostedAuthConfig } from "@/lib/api/operator";
import { createHostedAuthAppSchema } from "@/lib/api/schemas";
import type { Env } from "@/lib/env-shared";
import { formatDate } from "@/lib/format";
import { cn } from "@/lib/utils";
import { Button, LinkButton } from "@/components/ui/button";
import { Field, Input, Textarea } from "@/components/ui/field";
import { Dialog, DropdownMenu, ConfirmDialog } from "@/components/ui/overlay";
import { EmptyState, ErrorState, Alert, SuccessPanel } from "@/components/ui/feedback";
import { CodeBlock } from "@/components/ui/code-block";
import { Badge, EnvBadge } from "@/components/ui/badge";
import { useEnv } from "@/components/console/env-provider";
import { useToast } from "@/components/ui/toast";
import {
  AppIcon,
  ArrowRightIcon,
  EditIcon,
  KeyIcon,
  KebabIcon,
  PlusIcon,
  TrashIcon,
} from "@/components/ui/icons";
import {
  createAppAction,
  disableAppAction,
  rotateSecretAction,
  updateRedirectURIsAction,
  type AppActionState,
} from "./apps-actions";

const initialState: AppActionState = { ok: false };

function splitRedirectUris(value: string): string[] {
  return value
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
}

function formErrors(formData: FormData): Record<string, string> {
  const parsed = createHostedAuthAppSchema.safeParse({
    name: formData.get("name"),
    redirect_uris: splitRedirectUris(String(formData.get("redirect_uris") ?? "")),
  });
  if (parsed.success) return {};
  const next: Record<string, string> = {};
  for (const issue of parsed.error.issues) {
    const key = issue.path[0] ? String(issue.path[0]) : "form";
    if (!next[key]) next[key] = issue.message;
  }
  return next;
}

function focusFirstError(errors: Record<string, string>) {
  const first = Object.keys(errors)[0];
  if (first) {
    requestAnimationFrame(() => document.getElementById(first)?.focus());
  }
}

export function ApplicationsClient({
  providerId,
  env,
  apps,
  loadError,
}: {
  providerId: string | null;
  env: Env;
  apps: HostedAuthConfig[];
  loadError: string | null;
}) {
  const router = useRouter();
  const { env: activeEnv } = useEnv();
  const prevEnv = useRef(env);
  const [createOpen, setCreateOpen] = useState(false);
  const [createNonce, setCreateNonce] = useState(0);
  const [editing, setEditing] = useState<HostedAuthConfig | null>(null);
  const [rotating, setRotating] = useState<HostedAuthConfig | null>(null);
  const [deleting, setDeleting] = useState<HostedAuthConfig | null>(null);

  // R5：切换环境只改 URL/数据，不改子路由；检测到环境变化后由 RSC 重取列表。
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
          <h1 className="text-2xl font-semibold tracking-tight">应用</h1>
          <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
            管理你的 OIDC 应用：每个应用是一个“接入点”，告诉 vLogBin 你的产品是谁。
            当前环境为 {env === "test" ? "测试环境（沙箱）" : "生产环境（真实客户生效）"}。
          </p>
        </div>
        {providerId && (
          <Button onClick={openCreate}>
            <PlusIcon size={16} aria-hidden="true" />
            创建应用
          </Button>
        )}
      </header>

      {loadError ? (
        <ErrorState
          title="应用列表加载失败"
          description={loadError}
          action={
            <Button variant="outline" onClick={() => router.refresh()}>
              重试
            </Button>
          }
        />
      ) : !providerId ? (
        <EmptyState
          icon={<AppIcon size={20} aria-hidden="true" />}
          title="还没有可管理的 workspace"
          description="先创建并激活 Provider，获得测试环境后即可创建第一个 OIDC 应用。"
          action={
            <LinkButton href="/ops" variant="primary" prefetch={false}>
              前往 Provider
              <ArrowRightIcon size={16} aria-hidden="true" />
            </LinkButton>
          }
        />
      ) : apps.length === 0 ? (
        <EmptyState
          icon={<AppIcon size={20} aria-hidden="true" />}
          title="还没有 OIDC 应用"
          description={`在${env === "test" ? "测试环境" : "生产环境"}创建第一个应用，配置回调地址后即可接入登录。`}
          action={
            <Button onClick={openCreate}>
              <PlusIcon size={16} aria-hidden="true" />
              创建第一个应用
            </Button>
          }
        />
      ) : (
        <div className="overflow-x-auto rounded-xl border border-border bg-surface-1">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border bg-surface-2 text-left text-xs font-medium text-muted-foreground">
                <th className="px-4 py-3 font-medium">应用</th>
                <th className="px-4 py-3 font-medium">Client ID</th>
                <th className="px-4 py-3 font-medium">回调地址</th>
                <th className="px-4 py-3 font-medium">状态</th>
                <th className="px-4 py-3 font-medium">创建时间</th>
                <th className="px-4 py-3 font-medium" aria-label="操作" />
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {apps.map((app) => (
                <tr key={app.id} className="transition-colors hover:bg-surface-2/60">
                  <td className="px-4 py-3">
                    <span className="font-medium text-foreground">{app.name}</span>
                  </td>
                  <td className="px-4 py-3 font-mono text-xs text-muted-foreground tabular-nums">
                    {app.client_id}
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex max-w-md flex-wrap gap-1.5">
                      {app.redirect_uris.length === 0 ? (
                        <span className="text-muted-foreground">—</span>
                      ) : (
                        app.redirect_uris.map((uri) => (
                          <code
                            key={uri}
                            className="rounded-md bg-surface-2 px-1.5 py-0.5 font-mono text-[11px] text-foreground"
                          >
                            {uri}
                          </code>
                        ))
                      )}
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    <Badge variant={app.enabled ? "success" : "neutral"}>
                      <span
                        className={cn(
                          "inline-block size-1.5 rounded-full",
                          app.enabled ? "bg-success" : "bg-muted-foreground",
                        )}
                      />
                      {app.enabled ? "已启用" : "已禁用"}
                    </Badge>
                    <EnvBadge env={env} />
                  </td>
                  <td className="px-4 py-3 text-xs text-muted-foreground tabular-nums">
                    {formatDate(app.created_at)}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <DropdownMenu
                      align="end"
                      triggerLabel={`${app.name} 操作`}
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
                              编辑回调地址
                            </span>
                          ),
                          onSelect: () => setEditing(app),
                        },
                        {
                          type: "item",
                          label: (
                            <span className="inline-flex items-center gap-2">
                              <KeyIcon size={14} aria-hidden="true" />
                              轮换客户端密钥
                            </span>
                          ),
                          onSelect: () => setRotating(app),
                        },
                        { type: "separator" },
                        {
                          type: "item",
                          label: (
                            <span className="inline-flex items-center gap-2">
                              <TrashIcon size={14} aria-hidden="true" />
                              删除应用
                            </span>
                          ),
                          danger: true,
                          onSelect: () => setDeleting(app),
                        },
                      ]}
                    />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {providerId && (
        <>
          <CreateAppDialog
            key={`create-${createNonce}`}
            open={createOpen}
            onOpenChange={setCreateOpen}
            providerId={providerId}
            env={env}
          />

          {editing && (
            <EditRedirectsDialog
              key={`edit-${editing.id}`}
              open
              onOpenChange={(open) => {
                if (!open) setEditing(null);
              }}
              app={editing}
              providerId={providerId}
              env={env}
            />
          )}

          {rotating && (
            <RotateSecretDialog
              key={`rotate-${rotating.id}`}
              open
              onOpenChange={(open) => {
                if (!open) setRotating(null);
              }}
              app={rotating}
              providerId={providerId}
              env={env}
            />
          )}

          {deleting && (
            <DisableAppDialog
              key={`disable-${deleting.id}`}
              open
              onOpenChange={(open) => {
                if (!open) setDeleting(null);
              }}
              app={deleting}
              providerId={providerId}
              env={env}
            />
          )}
        </>
      )}
    </div>
  );
}

/* ---------------- 创建应用 ---------------- */
function CreateAppDialog({
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
  const [state, formAction, pending] = useActionState(createAppAction, initialState);
  const [errors, setErrors] = useState<Record<string, string>>({});

  useEffect(() => {
    if (state.ok) router.refresh();
  }, [state.ok, router]);

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title="创建 OIDC 应用"
      description={`应用创建后立即在${env === "test" ? "测试环境" : "生产环境"}生效。`}
      size="md"
    >
      {state.ok && state.app ? (
        <div className="space-y-4">
          <SuccessPanel
            title="应用创建成功"
            description={`${state.app.name} 已接入 ${env === "test" ? "测试" : "生产"}环境。Client ID 如下，请复制到你的应用中。`}
          >
            <div className="mt-3">
              <CodeBlock code={state.app.client_id} language="client_id" dense />
            </div>
          </SuccessPanel>
          <div className="flex flex-wrap items-center justify-end gap-3">
            <Button variant="ghost" onClick={() => onOpenChange(false)}>
              返回应用列表
            </Button>
            <LinkButton
              href="/console"
              variant="primary"
              prefetch={false}
              onClick={() => onOpenChange(false)}
            >
              返回概览
              <ArrowRightIcon size={16} aria-hidden="true" />
            </LinkButton>
          </div>
        </div>
      ) : (
        <form
          action={(formData) => {
            const next = formErrors(formData);
            setErrors(next);
            if (Object.keys(next).length === 0) formAction(formData);
            else focusFirstError(next);
          }}
          className="space-y-4"
          noValidate
        >
          <input type="hidden" name="provider_id" value={providerId} />
          <input type="hidden" name="env" value={env} />
          {state.error && (
            <Alert variant="danger" title="创建失败">
              {state.error}
            </Alert>
          )}
          <Field label="应用名称" htmlFor="name" error={errors.name}>
            <Input
              id="name"
              name="name"
              placeholder="例如：acme-web"
              autoFocus
              autoComplete="off"
              invalid={Boolean(errors.name)}
            />
          </Field>
          <Field
            label="回调地址"
            htmlFor="redirect_uris"
            hint="每行一个，例如 https://app.acme.com/callback"
            error={errors.redirect_uris}
          >
            <Textarea
              id="redirect_uris"
              name="redirect_uris"
              rows={4}
              placeholder={"https://app.acme.com/callback\nhttps://app.acme.com/oidc/callback"}
              invalid={Boolean(errors.redirect_uris)}
            />
          </Field>
          <div className="flex justify-end gap-3">
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" loading={pending}>
              <PlusIcon size={16} aria-hidden="true" />
              创建应用
            </Button>
          </div>
        </form>
      )}
    </Dialog>
  );
}

/* ---------------- 编辑回调地址 ---------------- */
function EditRedirectsDialog({
  open,
  onOpenChange,
  app,
  providerId,
  env,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  app: HostedAuthConfig;
  providerId: string;
  env: Env;
}) {
  const router = useRouter();
  const [state, formAction, pending] = useActionState(updateRedirectURIsAction, initialState);
  const [errors, setErrors] = useState<Record<string, string>>({});

  useEffect(() => {
    if (state.ok) router.refresh();
  }, [state.ok, router]);

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={`编辑回调地址 · ${app.name}`}
      description="更新 OIDC 回调地址不会更换客户端密钥。"
      size="md"
    >
      {state.ok ? (
        <div className="space-y-4">
          <SuccessPanel title="回调地址已更新">
            {state.app?.redirect_uris.map((uri) => (
              <code key={uri} className="block rounded-md bg-surface-2 px-2 py-1 font-mono text-xs">
                {uri}
              </code>
            ))}
          </SuccessPanel>
          <div className="flex justify-end">
            <Button variant="primary" onClick={() => onOpenChange(false)}>
              完成
            </Button>
          </div>
        </div>
      ) : (
        <form
          action={(formData) => {
            const next = formErrors(formData);
            setErrors(next);
            if (Object.keys(next).length === 0) formAction(formData);
            else focusFirstError(next);
          }}
          className="space-y-4"
          noValidate
        >
          <input type="hidden" name="provider_id" value={providerId} />
          <input type="hidden" name="env" value={env} />
          {state.error && (
            <Alert variant="danger" title="更新失败">
              {state.error}
            </Alert>
          )}
          <input type="hidden" name="name" value={app.name} />
          <Field
            label="回调地址"
            htmlFor="redirect_uris"
            hint="每行一个；修改后需要同步到你的 OIDC 客户端配置。"
            error={errors.redirect_uris}
          >
            <Textarea
              id="redirect_uris"
              name="redirect_uris"
              rows={5}
              defaultValue={app.redirect_uris.join("\n")}
              invalid={Boolean(errors.redirect_uris)}
            />
          </Field>
          <div className="flex justify-end gap-3">
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" loading={pending}>
              保存回调地址
            </Button>
          </div>
        </form>
      )}
    </Dialog>
  );
}

/* ---------------- 轮换客户端密钥 ---------------- */
function RotateSecretDialog({
  open,
  onOpenChange,
  app,
  providerId,
  env,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  app: HostedAuthConfig;
  providerId: string;
  env: Env;
}) {
  const router = useRouter();
  const { toast } = useToast();
  const [state, formAction, pending] = useActionState(rotateSecretAction, initialState);
  const [confirmTyped, setConfirmTyped] = useState("");
  const [copied, setCopied] = useState(false);

  const live = env === "live";

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={`轮换客户端密钥 · ${app.name}`}
      description={
        live
          ? "生产环境轮换后旧密钥立即失效，请确保客户端已切换新密钥。"
          : "轮换后旧密钥立即失效，请同步更新客户端配置。"
      }
      size="md"
    >
      {state.ok && state.secret ? (
        <div className="space-y-4">
          <SuccessPanel
            title="新密钥已生成"
            description="此密钥仅展示一次，离开本弹窗后将无法再次查看。"
          >
            <div className="mt-3">
              <CodeBlock
                code={state.secret}
                language="client_secret"
                dense
                onCopied={() => {
                  setCopied(true);
                  toast({ variant: "success", title: "已复制" });
                }}
              />
            </div>
          </SuccessPanel>
          <div className="flex flex-wrap items-center justify-end gap-3">
            <Button
              variant="ghost"
              onClick={() => {
                if (!copied)
                  toast({
                    variant: "info",
                    title: "密钥尚未复制",
                    description: "关闭后无法再次查看，建议先复制。",
                  });
                onOpenChange(false);
              }}
            >
              稍后复制
            </Button>
            <Button
              variant="primary"
              onClick={async () => {
                if (!state.secret) return;
                try {
                  await navigator.clipboard.writeText(state.secret);
                } catch {
                  // 复制失败不阻塞关闭；用户仍可手动复制。
                }
                setCopied(true);
                toast({ variant: "success", title: "已复制" });
                onOpenChange(false);
                router.refresh();
              }}
            >
              复制并完成
            </Button>
          </div>
        </div>
      ) : (
        <form action={formAction} className="space-y-4">
          <input type="hidden" name="provider_id" value={providerId} />
          <input type="hidden" name="env" value={env} />
          {state.error && (
            <Alert variant="danger" title="轮换失败">
              {state.error}
            </Alert>
          )}
          {live ? (
            <Field
              label="输入应用名称确认"
              htmlFor="rotate-confirm"
              hint={`输入 ${app.name} 后开始轮换。`}
            >
              <Input
                id="rotate-confirm"
                value={confirmTyped}
                onChange={(e) => setConfirmTyped(e.target.value)}
                placeholder={app.name}
                autoComplete="off"
                invalid={Boolean(confirmTyped) && confirmTyped !== app.name}
              />
            </Field>
          ) : (
            <Alert variant="warning" title="轮换后旧密钥立即失效">
              当前客户端若仍在使用旧密钥，将无法继续登录。请确认客户端已支持更新密钥。
            </Alert>
          )}
          <div className="flex justify-end gap-3">
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button
              type="submit"
              loading={pending}
              disabled={live && confirmTyped !== app.name}
            >
              <KeyIcon size={16} aria-hidden="true" />
              轮换密钥
            </Button>
          </div>
        </form>
      )}
    </Dialog>
  );
}

/* ---------------- 删除应用 ---------------- */
function DisableAppDialog({
  open,
  onOpenChange,
  app,
  providerId,
  env,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  app: HostedAuthConfig;
  providerId: string;
  env: Env;
}) {
  const router = useRouter();
  const { toast } = useToast();
  const [state, formAction, pending] = useActionState(disableAppAction, initialState);

  useEffect(() => {
    if (state.ok) {
      toast({ variant: "success", title: "应用已删除" });
      router.refresh();
      onOpenChange(false);
    }
  }, [state.ok, router, onOpenChange, toast]);

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title={`删除应用 · ${app.name}`}
      description={`删除后 ZITADEL 项目一并移除，${env === "test" ? "测试环境" : "生产环境"}中依赖该应用的登录立即失效，此操作不可撤销。`}
      confirmLabel="删除应用"
      confirmText={app.name}
      pending={pending}
      onConfirm={() => {
        const formData = new FormData();
        formData.set("provider_id", providerId);
        formData.set("env", env);
        startTransition(() => formAction(formData));
      }}
    />
  );
}
