"use client";

import { useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import type { WorkspaceMembership } from "@/lib/api/operator";
import { formatDate } from "@/lib/format";
import { Button, LinkButton } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/field";
import { Dialog, ConfirmDialog } from "@/components/ui/overlay";
import {
  Alert,
  EmptyState,
  ErrorState,
} from "@/components/ui/feedback";
import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { useActionFeedback } from "@/hooks/use-action-feedback";
import {
  ArrowRightIcon,
  EditIcon,
  PlusIcon,
  TrashIcon,
  UsersIcon,
} from "@/components/ui/icons";
import {
  inviteMemberAction,
  removeMemberAction,
  updateMemberRoleAction,
  type MemberActionState,
} from "./users-actions";

const initialState: MemberActionState = { ok: false };

const ROLES = [
  { value: "provider_admin", label: "provider_admin" },
  { value: "provider_developer", label: "provider_developer" },
  { value: "provider_billing", label: "provider_billing" },
];

const ROLE_VARIANT: Record<string, "success" | "info" | "warning" | "neutral"> = {
  provider_admin: "success",
  provider_developer: "info",
  provider_billing: "warning",
};

export function UsersClient({
  workspaceId,
  workspaceName,
  members,
  loadError,
}: {
  workspaceId: string | null;
  workspaceName: string | null;
  members: WorkspaceMembership[];
  loadError: string | null;
}) {
  const router = useRouter();
  const [editor, setEditor] = useState<{
    member?: WorkspaceMembership;
  } | null>(null);
  const [removing, setRemoving] = useState<WorkspaceMembership | null>(null);

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Users</h1>
          <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
            管理工作区成员与平台角色。当前 workspace：{workspaceName ?? "—"}。
          </p>
        </div>
        {workspaceId && (
          <Button onClick={() => setEditor({})}>
            <PlusIcon size={16} aria-hidden="true" />
            邀请成员
          </Button>
        )}
      </header>

      {loadError ? (
        <ErrorState title="成员列表加载失败" description={loadError} />
      ) : !workspaceId ? (
        <EmptyState
          icon={<UsersIcon size={20} aria-hidden="true" />}
          title="还没有 workspace"
          description="注册后平台会自动创建默认 workspace 并授予当前用户管理员角色。"
          action={
            <LinkButton href="/ops" variant="primary" prefetch={false}>
              前往 Provider
              <ArrowRightIcon size={16} aria-hidden="true" />
            </LinkButton>
          }
        />
      ) : members.length === 0 ? (
        <EmptyState
          icon={<UsersIcon size={20} aria-hidden="true" />}
          title="暂无成员"
          description="邀请第一个协作成员加入 workspace。"
        />
      ) : (
        <Card className="overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="bg-surface-2/70 text-left text-xs font-medium text-muted-foreground">
                <tr>
                  <th className="px-4 py-3 font-medium">用户</th>
                  <th className="px-4 py-3 font-medium">角色</th>
                  <th className="px-4 py-3 font-medium">状态</th>
                  <th className="px-4 py-3 font-medium">加入时间</th>
                  <th className="px-4 py-3 text-right font-medium">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {members.map((member) => (
                  <tr
                    key={member.id}
                    className="transition-colors hover:bg-surface-2/60"
                  >
                    <td className="px-4 py-3">
                      <code className="font-mono text-xs text-foreground">
                        {member.user_sub}
                      </code>
                    </td>
                    <td className="px-4 py-3">
                      <Badge variant={ROLE_VARIANT[member.role] ?? "neutral"}>
                        {member.role}
                      </Badge>
                    </td>
                    <td className="px-4 py-3">
                      <Badge
                        variant={
                          member.status === "active" ? "success" : "neutral"
                        }
                      >
                        {member.status}
                      </Badge>
                    </td>
                    <td className="px-4 py-3 text-xs text-muted-foreground tabular-nums">
                      {formatDate(member.created_at)}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <div className="inline-flex items-center gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          aria-label={`编辑 ${member.user_sub}`}
                          onClick={() => setEditor({ member })}
                        >
                          <EditIcon size={14} aria-hidden="true" />
                          编辑
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          aria-label={`移除 ${member.user_sub}`}
                          onClick={() => setRemoving(member)}
                        >
                          <TrashIcon size={14} aria-hidden="true" />
                          移除
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Card>
      )}

      {editor && (
        <MemberDialog
          key={editor.member?.user_sub ?? "new"}
          workspaceId={workspaceId!}
          member={editor.member}
          onOpenChange={(open) => {
            if (!open) setEditor(null);
          }}
          onSaved={() => router.refresh()}
        />
      )}

      {removing && (
        <RemoveMemberDialog
          key={`remove-${removing.user_sub}`}
          workspaceId={workspaceId!}
          member={removing}
          onOpenChange={(open) => {
            if (!open) setRemoving(null);
          }}
          onRemoved={() => router.refresh()}
        />
      )}
    </div>
  );
}

function MemberDialog({
  workspaceId,
  member,
  onOpenChange,
  onSaved,
}: {
  workspaceId: string;
  member?: WorkspaceMembership;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
}) {
  const [userSub, setUserSub] = useState(member?.user_sub ?? "");
  const [role, setRole] = useState(member?.role ?? "provider_developer");
  const { state, formAction, pending } = useActionFeedback<MemberActionState>({
    action: member ? updateMemberRoleAction : inviteMemberAction,
    initialState,
    onSuccess: () => {
      onOpenChange(false);
      onSaved();
    },
    successTitle: member ? "角色已更新" : "成员已邀请",
  });

  function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const fd = new FormData();
    fd.set("workspace_id", workspaceId);
    fd.set("user_sub", userSub.trim());
    fd.set("role", role);
    formAction(fd);
  }

  return (
    <Dialog
      open
      onOpenChange={onOpenChange}
      title={member ? "更新成员角色" : "邀请成员"}
      description="成员通过平台 user_sub 加入 workspace。"
      size="md"
    >
      <form onSubmit={submit} className="space-y-4">
        <Field label="用户 subject" htmlFor="user_sub">
          <Input
            id="user_sub"
            value={userSub}
            disabled={Boolean(member)}
            onChange={(e) => setUserSub(e.target.value)}
            autoComplete="off"
            placeholder="operator"
          />
        </Field>
        <Field label="角色" htmlFor="role">
          <Select
            id="role"
            value={role}
            onChange={(e) => setRole(e.target.value)}
          >
            {ROLES.map((r) => (
              <option key={r.value} value={r.value}>
                {r.label}
              </option>
            ))}
          </Select>
        </Field>
        {state.error && <Alert title="保存失败">{state.error}</Alert>}
        <div className="flex justify-end gap-2">
          <Button variant="ghost" type="button" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button type="submit" loading={pending}>
            {member ? "保存修改" : "邀请成员"}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}

function RemoveMemberDialog({
  workspaceId,
  member,
  onOpenChange,
  onRemoved,
}: {
  workspaceId: string;
  member: WorkspaceMembership;
  onOpenChange: (open: boolean) => void;
  onRemoved: () => void;
}) {
  const { state, formAction, pending } = useActionFeedback<MemberActionState>({
    action: removeMemberAction,
    initialState,
    onSuccess: () => {
      onOpenChange(false);
      onRemoved();
    },
    successTitle: "成员已移除",
  });

  function confirm() {
    const fd = new FormData();
    fd.set("workspace_id", workspaceId);
    fd.set("user_sub", member.user_sub);
    formAction(fd);
  }

  return (
    <ConfirmDialog
      open
      onOpenChange={onOpenChange}
      title="移除成员"
      description={
        <div className="space-y-2">
          <p>
            将移除 <code className="font-mono text-xs">{member.user_sub}</code>。
            输入用户 subject 确认。
          </p>
          {state.error && <Alert title="移除失败">{state.error}</Alert>}
        </div>
      }
      confirmText={member.user_sub}
      pending={pending}
      onConfirm={confirm}
      confirmLabel="移除成员"
    />
  );
}
