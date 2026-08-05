import { z } from "zod";

/**
 * 平台 operator API 响应校验（zod，运行时）。
 * 实体类型统一 re-export 自 lib/api/types.ts（§11 变更管理：前端类型以
 * docs/openapi.yaml 为准，本文件仅提供运行时校验，不再定义同名类型）。
 */

export const providerSchema = z.object({
  id: z.string(),
  slug: z.string(),
  name: z.string(),
  description: z.string().optional().nullable(),
  lifecycle_state: z.string(),
  home_region_code: z.string(),
  created_at: z.string().optional().nullable(),
  updated_at: z.string().optional().nullable(),
});

export const environmentSchema = z.object({
  id: z.string(),
  provider_id: z.string(),
  kind: z.string(),
  status: z.string(),
  issuer: z.string().optional().nullable(),
  created_at: z.string().optional().nullable(),
});

export const regionSchema = z.object({
  code: z.string(),
  name: z.string(),
  enabled: z.boolean().optional().default(true),
});

export const lifecycleResultSchema = z.object({
  provider: providerSchema.nullable(),
  environment: environmentSchema.nullable(),
  api_key: z.string().nullable().optional(),
});

export const createProviderInputSchema = z.object({
  slug: z
    .string()
    .trim()
    .min(1, "Slug 不能为空")
    .max(63, "Slug 过长")
    .regex(
      /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/,
      "Slug 只能包含小写字母、数字与中划线，且不能以中划线开头/结尾",
    ),
  name: z.string().trim().min(1, "名称不能为空").max(80, "名称过长"),
  home_region_code: z.string().trim().min(1, "请选择所属区域"),
  description: z.string().trim().max(500, "描述过长").optional(),
});

export const activateProviderInputSchema = z.object({
  provider_id: z.string().trim().min(1, "缺少 Provider ID"),
  home_region_code: z.string().trim().min(1, "请选择所属区域"),
  reason: z.string().trim().max(500, "操作原因过长（最多 500 字）").optional(),
});

/** 生命周期转换的操作原因（可选），最长 500 字。 */
export const lifecycleReasonSchema = z
  .string()
  .trim()
  .max(500, "操作原因过长（最多 500 字）")
  .optional();

export const lifecycleTargetSchema = z.enum([
  "LIVE_REVIEW",
  "LIVE_ACTIVE",
  "RESTRICTED",
  "SUSPENDED",
]);

export const loginFormSchema = z.object({
  token: z
    .string()
    .trim()
    .min(8, "令牌太短")
    .regex(/^[A-Za-z0-9._\-]+$/, "令牌包含非法字符"),
});

/** 创建 OIDC 应用的入参（Applications 页面）。 */
export const createHostedAuthAppSchema = z.object({
  name: z.string().trim().min(1, "应用名称不能为空").max(80, "应用名称过长"),
  redirect_uris: z
    .array(z.string().trim().url("请输入合法回调地址（https://…）"))
    .min(1, "至少需要一个回调地址"),
});

/** 创建客户入参（Customers 页面）。 */
export const createCustomerSchema = z.object({
  external_id: z
    .string()
    .trim()
    .min(1, "客户外部 ID 不能为空")
    .max(120, "客户外部 ID 过长"),
  account_type: z.enum(["individual", "business"], "请选择客户类型"),
  display_name: z.string().trim().min(1, "客户名称不能为空").max(120, "客户名称过长"),
});

/** 价格入参运行时校验（properties 结构随 charge_model 变化）。 */
export const priceInputSchema = z
  .object({
    metric_code: z.string().trim().optional(),
    charge_model: z.enum(["fixed", "per_unit", "tiered"]),
    properties: z.record(z.string(), z.unknown()).default({}),
  })
  .superRefine((value, ctx) => {
    const props = value.properties ?? {};
    if (value.charge_model === "fixed") {
      if (
        typeof props.amount_cents !== "number" ||
        !Number.isInteger(props.amount_cents) ||
        props.amount_cents < 0
      ) {
        ctx.addIssue({
          code: "custom",
          path: ["properties", "amount_cents"],
          message: "固定价格金额必须为非负整数（分）",
        });
      }
    }
    if (value.charge_model === "per_unit") {
      if (!value.metric_code) {
        ctx.addIssue({
          code: "custom",
          path: ["metric_code"],
          message: "按量计费必须选择指标",
        });
      }
      if (
        typeof props.unit_amount_cents !== "number" ||
        !Number.isInteger(props.unit_amount_cents) ||
        props.unit_amount_cents < 0
      ) {
        ctx.addIssue({
          code: "custom",
          path: ["properties", "unit_amount_cents"],
          message: "单价必须为非负整数（分）",
        });
      }
    }
    if (value.charge_model === "tiered") {
      if (!value.metric_code) {
        ctx.addIssue({
          code: "custom",
          path: ["metric_code"],
          message: "阶梯计费必须选择指标",
        });
      }
      const tiers = props.tiers;
      if (!Array.isArray(tiers) || tiers.length === 0) {
        ctx.addIssue({
          code: "custom",
          path: ["properties", "tiers"],
          message: "阶梯计费至少需要一个区间",
        });
      }
    }
  });

/** 权益入参校验（本期 Policies 页面管理，PlanInput 中可选）。 */
export const entitlementInputSchema = z.object({
  key: z.string().trim().min(1, "权益 key 不能为空"),
  value_type: z.enum(["boolean", "numeric", "period"]),
  value: z.unknown(),
});

/** 套餐创建/更新入参校验（服务端仍执行完整目录结构校验）。 */
export const planInputSchema = z.object({
  code: z
    .string()
    .trim()
    .min(1, "套餐代码不能为空")
    .max(63, "套餐代码过长")
    .regex(
      /^[a-z0-9](?:[a-z0-9_-]{0,61}[a-z0-9])?$/,
      "代码只能包含小写字母、数字、中划线和下划线，且不能以中划线/下划线开头结尾",
    ),
  name: z.string().trim().min(1, "套餐名称不能为空").max(80, "套餐名称过长"),
  interval: z.enum(["weekly", "monthly", "yearly"], "请选择计费周期"),
  currency: z
    .string()
    .trim()
    .length(3, "货币代码必须为 3 位字母")
    .regex(/^[A-Z]{3}$/, "货币代码必须为大写字母"),
  prices: z.array(priceInputSchema).min(1, "至少需要一个价格"),
  entitlements: z.array(entitlementInputSchema).optional(),
});

import type {
  Provider,
  Environment,
  Region,
  LifecycleResult,
  CreateProviderInput,
  LifecycleTarget,
  PlanInput,
  PriceInput,
  EntitlementInput,
  CustomerCreateInput,
} from "./types";

export type {
  Provider,
  Environment,
  Region,
  LifecycleResult,
  CreateProviderInput,
  LifecycleTarget,
  PlanInput,
  PriceInput,
  EntitlementInput,
  CustomerCreateInput,
};
