export interface OnboardingStep {
  id: string;
  title: string;
  description: string;
  href: string;
  done: boolean;
}

export interface OnboardingState {
  steps: OnboardingStep[];
  progress: number; // 0-100
  completed: boolean;
}

/**
 * First-Run 引导：基于真实数据推断使用进度。
 * §2.2 四步模型（M2 回填）：创建应用 → 创建套餐 → 创建客户 → 上报用量，
 * 全部完成后“10 分钟上线”的计费闭环打通。
 */
export function getOnboardingState(input: {
  appCount: number;
  planCount: number;
  customerCount: number;
  usageEventCount: number;
}): OnboardingState {
  const steps: OnboardingStep[] = [
    {
      id: "application",
      title: "创建第一个应用",
      description: "配置 OIDC 回调地址，为你的产品接入身份认证。",
      href: "/console/identity/applications",
      done: input.appCount > 0,
    },
    {
      id: "plan",
      title: "创建第一个套餐",
      description: "定义订阅价格模型，形成可发布目录版本。",
      href: "/console/billing/plans",
      done: input.planCount > 0,
    },
    {
      id: "customer",
      title: "创建第一个客户",
      description: "创建终端客户，准备进入订阅与计费。",
      href: "/console/billing/customers",
      done: input.customerCount > 0,
    },
    {
      id: "usage",
      title: "上报第一条用量事件",
      description: "通过 SDK / API 上报计量事件，打通“应用→套餐→客户→用量”闭环。",
      href: "/console",
      done: input.usageEventCount > 0,
    },
  ];

  const doneCount = steps.filter((s) => s.done).length;
  return {
    steps,
    progress: Math.round((doneCount / steps.length) * 100),
    completed: doneCount === steps.length,
  };
}
