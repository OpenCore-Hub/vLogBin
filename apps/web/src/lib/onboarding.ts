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
 * 数据不足时引导用户完成首个价值闭环（创建 → 发布 → 上线）。
 */
export function getOnboardingState(input: {
  providerCount: number;
  activatedProviderCount: number;
  publishedVersionCount: number;
  liveActiveCount: number;
}): OnboardingState {
  const steps: OnboardingStep[] = [
    {
      id: "provider",
      title: "创建并激活 Provider",
      description:
        "创建服务提供商后分配区域并激活，获得测试环境与 API Key。",
      // 已有 Provider（REGISTERED 未激活）时引导去详情页激活，否则去新建。
      href: input.providerCount > 0 ? "/ops" : "/ops/new",
      done: input.activatedProviderCount > 0,
    },
    {
      id: "catalog",
      title: "发布目录版本",
      description: "定义指标（Metrics）与套餐（Plans），形成可订阅的目录版本。",
      href: "/ops",
      done: input.publishedVersionCount > 0,
    },
    {
      id: "live",
      title: "上线生产环境",
      description: "发起生命周期审核，将 Provider 推进至生产环境。",
      href: "/ops",
      done: input.liveActiveCount > 0,
    },
  ];

  const doneCount = steps.filter((s) => s.done).length;
  return {
    steps,
    progress: Math.round((doneCount / steps.length) * 100),
    completed: doneCount === steps.length,
  };
}
