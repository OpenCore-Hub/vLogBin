import type { Metadata } from "next";
import Link from "next/link";
import { Logo } from "@/components/brand/logo";
import { LinkButton } from "@/components/ui/button";
import { TerminalHero } from "@/components/marketing/terminal-hero";
import {
  BoxIcon,
  CheckIcon,
  CreditCardIcon,
  GlobeIcon,
  ShieldIcon,
  TerminalIcon,
  UsersIcon,
  ZapIcon,
} from "@/components/ui/icons";
import { getSession } from "@/lib/auth/session";

export const metadata: Metadata = {
  title: "vLogBin — 为开发者打造的计量计费平台",
  description:
    "vLogBin 以账单即代码为理念，为云原生服务提供用量计量、套餐目录与订阅计费的一体化基础设施。",
};

const FEATURES = [
  {
    icon: <ZapIcon size={18} />,
    title: "用量计量",
    description: "一条 API 上报任意维度的用量事件，实时聚合、精确到毫秒。",
  },
  {
    icon: <CreditCardIcon size={18} />,
    title: "套餐目录",
    description: "以版本化目录管理套餐与价格，发布即生效，回滚有记录。",
  },
  {
    icon: <UsersIcon size={18} />,
    title: "订阅计费",
    description: "客户订阅、账单生成、额度执行，全生命周期自动化。",
  },
  {
    icon: <ShieldIcon size={18} />,
    title: "鉴权与审计",
    description: "内置 OIDC 身份治理与完整审计轨迹，合规开箱即用。",
  },
];

const STEPS = [
  {
    n: "01",
    title: "注册工作空间",
    description: "几分钟内完成身份认证，获得独立的 test / live 双环境。",
  },
  {
    n: "02",
    title: "定义你的套餐",
    description: "用代码描述价格模型，发布到目录，客户即可订阅。",
  },
  {
    n: "03",
    title: "上报用量，收取账单",
    description: "接入 SDK 上报用量事件，剩余额度与账单自动生成。",
  },
];

export default async function HomePage() {
  const session = await getSession();

  return (
    <div className="min-h-dvh bg-canvas">
      {/* 悬浮玻璃导航岛 */}
      <header className="sticky top-0 z-40 px-4 pt-4">
        <div className="mx-auto flex max-w-5xl items-center justify-between rounded-full border border-border bg-surface-1/85 py-2 pl-5 pr-2 shadow-[var(--shadow-premium)] backdrop-blur-md">
          <Link href="/" aria-label="vLogBin 首页">
            <Logo />
          </Link>
          <nav className="hidden items-center gap-1 text-sm text-muted-foreground sm:flex">
            <Link href="#features" className="rounded-full px-3 py-1.5 transition-colors hover:bg-surface-2 hover:text-foreground">
              能力
            </Link>
            <Link href="#how" className="rounded-full px-3 py-1.5 transition-colors hover:bg-surface-2 hover:text-foreground">
              工作方式
            </Link>
            <Link href="#cta" className="rounded-full px-3 py-1.5 transition-colors hover:bg-surface-2 hover:text-foreground">
              开始使用
            </Link>
          </nav>
          <div className="flex items-center gap-1.5">
            {session ? (
              <LinkButton href="/console">进入控制台</LinkButton>
            ) : (
              <>
                <LinkButton href="/login" variant="ghost" className="rounded-full px-4">
                  登录
                </LinkButton>
                <LinkButton href="/signup" variant="primary" className="px-5">
                  免费开始
                </LinkButton>
              </>
            )}
          </div>
        </div>
      </header>

      {/* Hero：非对称编辑式布局 */}
      <section className="mx-auto max-w-7xl px-6 pb-28 pt-24 sm:pt-32">
        <div className="grid items-center gap-14 lg:grid-cols-[1.05fr_0.95fr]">
          <div className="space-y-8">
            <span className="animate-reveal inline-flex items-center gap-2 rounded-full border border-border bg-surface-1 px-4 py-1.5 text-[11px] font-medium uppercase tracking-[0.2em] text-brand-700 shadow-[var(--shadow-sm)]">
              <TerminalIcon size={13} />
              计费即代码 · Billing as Code
            </span>
            <h1 className="animate-reveal-delay-1 text-balance text-[2.75rem] font-semibold leading-[1.04] tracking-[-0.035em] text-foreground sm:text-6xl lg:text-[4.5rem]">
              用量、套餐、账单，
              <br />
              <span className="text-brand-700">一次 API 全部搞定</span>
            </h1>
            <p className="animate-reveal-delay-2 max-w-lg text-lg leading-relaxed text-muted-foreground">
              vLogBin 为云原生服务提供计量计费基础设施：用量上报、套餐目录、
              订阅与账单自动化。开发者只关心业务，计费交给 vLogBin。
            </p>
            <div className="animate-reveal-delay-2 flex flex-wrap items-center gap-3">
              <LinkButton href="/signup" size="lg">
                免费开始
              </LinkButton>
              <LinkButton href="/login" size="lg" variant="secondary">
                查看演示
              </LinkButton>
            </div>
            <ul className="animate-reveal-delay-3 flex flex-wrap gap-x-5 gap-y-2 text-sm text-muted-foreground">
              {["30 秒接入", "test / live 隔离", "OIDC 原生"].map((t) => (
                <li key={t} className="flex items-center gap-2">
                  <span className="flex size-5 items-center justify-center rounded-full bg-success-soft">
                    <CheckIcon size={12} className="text-success" />
                  </span>
                  {t}
                </li>
              ))}
            </ul>
          </div>
          <div className="animate-reveal-delay-2">
            <TerminalHero />
          </div>
        </div>
      </section>

      {/* 能力：非对称 Bento */}
      <section id="features" className="relative border-t border-border bg-surface-1">
        <div className="mx-auto max-w-7xl px-6 py-28">
          <div className="mb-16 max-w-2xl">
            <p className="mb-3 text-[11px] font-medium uppercase tracking-[0.2em] text-muted-foreground">
              Platform
            </p>
            <h2 className="text-balance text-3xl font-semibold tracking-[-0.025em] text-foreground sm:text-4xl">
              计量、计费、治理，一个平台
            </h2>
            <p className="mt-4 max-w-xl text-base leading-relaxed text-muted-foreground">
              从第一笔用量到年终账单，每个环节都可观测、可审计、可回滚。
            </p>
          </div>
          <div className="grid gap-4 md:grid-cols-6">
            {FEATURES.map((f, i) => {
              const spans = [
                "md:col-span-3 lg:col-span-2",
                "md:col-span-3 lg:col-span-2",
                "md:col-span-3 lg:col-span-1",
                "md:col-span-3 lg:col-span-1",
              ];
              return (
                <div
                  key={f.title}
                  className={`group rounded-2xl bg-surface-2 p-1.5 transition-transform duration-700 ease-[var(--ease-premium)] hover:-translate-y-1 ${spans[i]}`}
                >
                  <div className="surface-premium h-full rounded-[1.25rem] p-6">
                    <span className="mb-5 inline-flex size-10 items-center justify-center rounded-full bg-brand-700 text-white shadow-[var(--shadow-premium)] transition-transform duration-500 ease-[var(--ease-premium)] group-hover:scale-105">
                      {f.icon}
                    </span>
                    <h3 className="text-base font-semibold text-foreground">
                      {f.title}
                    </h3>
                    <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
                      {f.description}
                    </p>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      </section>

      {/* 工作方式：编辑式分割 */}
      <section id="how" className="mx-auto max-w-7xl px-6 py-28">
        <div className="mb-16 flex items-end justify-between gap-6">
          <div>
            <p className="mb-3 text-[11px] font-medium uppercase tracking-[0.2em] text-muted-foreground">
              Workflow
            </p>
            <h2 className="text-3xl font-semibold tracking-[-0.025em] text-foreground sm:text-4xl">
              三步接入
            </h2>
          </div>
          <span className="hidden size-12 items-center justify-center rounded-full bg-terminal-bg text-terminal-fg shadow-[var(--shadow-premium)] sm:flex">
            <BoxIcon size={20} />
          </span>
        </div>
        <div className="grid gap-5 md:grid-cols-3">
          {STEPS.map((s) => (
            <div key={s.n} className="group rounded-2xl bg-surface-2 p-1.5 transition-transform duration-700 ease-[var(--ease-premium)] hover:-translate-y-1">
              <div className="surface-premium relative overflow-hidden rounded-[1.25rem] p-7">
                <span className="pointer-events-none absolute -right-2 -top-6 font-mono text-[7rem] font-semibold leading-none text-brand-700/10">
                  {s.n}
                </span>
                <span className="relative inline-flex h-8 items-center rounded-full border border-border bg-surface-2 px-3 font-mono text-xs text-brand-700">
                  {s.n}
                </span>
                <h3 className="relative mt-6 text-lg font-semibold text-foreground">
                  {s.title}
                </h3>
                <p className="relative mt-2 text-sm leading-relaxed text-muted-foreground">
                  {s.description}
                </p>
              </div>
            </div>
          ))}
        </div>
      </section>

      {/* CTA */}
      <section id="cta" className="px-4 pb-28">
        <div className="mx-auto max-w-7xl">
          <div className="relative overflow-hidden rounded-2xl bg-terminal-bg p-1.5 shadow-[0_32px_96px_-32px_rgba(0,0,0,0.55)] ring-1 ring-white/10">
            <div className="flex flex-col items-center gap-6 rounded-2xl border border-white/10 bg-terminal-bg px-6 py-20 text-center">
              <span className="flex size-14 items-center justify-center rounded-full bg-white/5 text-terminal-fg ring-1 ring-white/10">
                <GlobeIcon size={24} />
              </span>
              <h2 className="max-w-2xl text-balance text-3xl font-semibold tracking-[-0.02em] text-terminal-fg sm:text-4xl">
                开始你的第一个计费闭环
              </h2>
              <p className="max-w-xl text-base text-terminal-muted">
                无需信用卡。注册后即可获得隔离的 test 环境，30 秒完成首次用量上报。
              </p>
              <LinkButton
                href="/signup"
                size="lg"
                className="rounded-full bg-terminal-fg px-7 text-terminal-bg shadow-[0_12px_40px_-12px_rgba(94,234,212,0.55)] hover:bg-white"
              >
                创建工作空间
              </LinkButton>
            </div>
          </div>
        </div>
      </section>

      {/* 页脚 */}
      <footer className="border-t border-border bg-surface-1">
        <div className="mx-auto flex max-w-7xl flex-col items-center justify-between gap-4 px-6 py-10 sm:flex-row">
          <Logo size={18} />
          <p className="text-xs text-muted-foreground">
            © {new Date().getFullYear()} vLogBin · Billing as Code
          </p>
        </div>
      </footer>
    </div>
  );
}
