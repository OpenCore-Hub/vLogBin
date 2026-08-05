"use client";

import { useActionState, useState } from "react";
import { createProviderAction, type OpActionState } from "../actions";
import { createProviderInputSchema } from "@/lib/api/schemas";
import type { Region } from "@/lib/api/operator";
import { Field, Input, Select } from "@/components/ui/field";
import { Button, LinkButton } from "@/components/ui/button";
import { Alert, SuccessPanel } from "@/components/ui/feedback";
import { CodeBlock } from "@/components/ui/code-block";
import { useToast } from "@/components/ui/toast";
import { ArrowRightIcon, KeyIcon, PlusIcon } from "@/components/ui/icons";

const initialState: OpActionState = { ok: false };

export function NewProviderForm({ regions }: { regions: Region[] }) {
  const [state, formAction, pending] = useActionState(createProviderAction, initialState);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [apiKeyCopied, setApiKeyCopied] = useState(false);
  const { toast } = useToast();

  function validate(formData: FormData) {
    const parsed = createProviderInputSchema.safeParse({
      slug: formData.get("slug"),
      name: formData.get("name"),
      home_region_code: formData.get("home_region_code"),
    });
    if (parsed.success) {
      setErrors({});
      return true;
    }
    const next: Record<string, string> = {};
    for (const issue of parsed.error.issues) {
      if (issue.path[0] && !next[String(issue.path[0])]) {
        next[String(issue.path[0])] = issue.message;
      }
    }
    setErrors(next);
    // 聚焦首个错误字段，减少用户找错成本（零摩擦抽查①）
    const firstField = Object.keys(next)[0];
    if (firstField) {
      requestAnimationFrame(() => {
        document.getElementById(firstField)?.focus();
      });
    }
    return false;
  }

  // 密钥仅展示一次：离开前若尚未复制，给一次性非阻塞提醒（零摩擦抽查④）
  function handleLeave() {
    if (state.apiKey && !apiKeyCopied) {
      toast({
        variant: "info",
        title: "API Key 尚未复制",
        description: "密钥仅展示一次，离开后将无法再次查看。",
      });
    }
  }

  /* ---------- 成功态：展示 API Key ---------- */
  if (state.ok) {
    return (
      <div className="space-y-6">
        <SuccessPanel
          title="Provider 创建成功"
          description={
            state.providerId
              ? "测试环境与沙箱已就绪。这是你的 API Key，仅展示一次，请妥善保管。"
              : "测试环境与沙箱已就绪。"
          }
        >
          {state.apiKey ? (
            <div className="mt-3">
              <div className="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-terminal-muted">
                <KeyIcon size={13} aria-hidden="true" />
                API Key
              </div>
              <CodeBlock
                code={state.apiKey}
                language="key"
                dense
                onCopied={() => setApiKeyCopied(true)}
              />
            </div>
          ) : null}
        </SuccessPanel>

        <div className="flex flex-wrap items-center gap-3">
          {state.providerId && (
            <LinkButton
              href={`/ops/${state.providerId}`}
              prefetch={false}
              variant="primary"
              onClick={handleLeave}
            >
              查看 Provider 详情
              <ArrowRightIcon size={16} aria-hidden="true" />
            </LinkButton>
          )}
          <LinkButton href="/ops" variant="outline" onClick={handleLeave} prefetch={false}>
            返回列表
          </LinkButton>
        </div>
      </div>
    );
  }

  /* ---------- 表单态 ---------- */
  return (
    <form
      action={(formData) => {
        if (validate(formData)) formAction(formData);
      }}
      className="space-y-5"
      noValidate
    >
      {state.error && (
        <Alert variant="danger" title="创建失败">
          {state.error}
        </Alert>
      )}

      <div className="space-y-5 rounded-xl border border-border bg-surface-1 p-5">
        <Field label="名称" htmlFor="name" error={errors.name}>
          <Input
            id="name"
            name="name"
            placeholder="例如：Acme Cloud"
            autoComplete="off"
            invalid={Boolean(errors.name)}
          />
        </Field>

        <Field
          label="Slug"
          htmlFor="slug"
          hint="小写字母、数字与中划线（-），全局唯一，创建后不可修改。"
          error={errors.slug}
        >
          <Input
            id="slug"
            name="slug"
            placeholder="例如：acme-cloud"
            autoComplete="off"
            autoCapitalize="none"
            spellCheck={false}
            invalid={Boolean(errors.slug)}
          />
        </Field>

        <Field
          label="所属区域"
          htmlFor="home_region_code"
          error={errors.home_region_code}
        >
          {regions.length > 0 ? (
            <Select
              id="home_region_code"
              name="home_region_code"
              defaultValue=""
              invalid={Boolean(errors.home_region_code)}
            >
              <option value="" disabled>
                请选择区域…
              </option>
              {regions.map((r) => (
                <option key={r.id} value={r.code}>
                  {r.code} · {r.jurisdiction}
                </option>
              ))}
            </Select>
          ) : (
            <Input
              id="home_region_code"
              name="home_region_code"
              placeholder="区域列表暂不可用，请输入区域代码"
              invalid={Boolean(errors.home_region_code)}
            />
          )}
        </Field>
      </div>

      <div className="flex items-center justify-end gap-3">
        <LinkButton href="/ops" variant="ghost" prefetch={false}>
          取消
        </LinkButton>
        <Button type="submit" variant="primary" loading={pending}>
          <PlusIcon size={16} aria-hidden="true" />
          创建 Provider
        </Button>
      </div>
    </form>
  );
}
