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
      {/* 顶部导航 */}
      <header className="sticky top-0 z-40 border-b border-border bg-canvas/85 backdrop-blur">
        <div className="mx-auto flex h-16 max-w-6xl items-center justify-between px-6">
          <Link href="/" aria-label="vLogBin 首页">
            <Logo />
          </Link>
          <nav className="hidden items-center gap-6 text-sm text-muted-foreground sm:flex">
            <Link href="#features" className="transition-colors hover:text-foreground">
              能力
            </Link>
            <Link href="#how" className="transition-colors hover:text-foreground">
              工作方式
            </Link>
            <Link href="#cta" className="transition-colors hover:text-foreground">
              开始使用
            </Link>
          </nav>
          <div className="flex items-center gap-2">
            {session ? (
              <LinkButton href="/console">进入控制台</LinkButton>
            ) : (
              <>
                <LinkButton href="/login" variant="ghost">
                  登录
                </LinkButton>
                <LinkButton href="/signup" variant="primary">
                  免费开始
                </LinkButton>
              </>
            )}
          </div>
        </div>
      </header>

      {/* Hero */}
      <section className="mx-auto max-w-6xl px-6 pt-20 pb-16 sm:pt-28">
        <div className="grid items-center gap-12 lg:grid-cols-2">
          <div className="space-y-6">
            <span className="inline-flex items-center gap-1.5 rounded-full border border-brand-200 bg-brand-50 px-3 py-1 text-xs font-medium text-brand-700">
              <TerminalIcon size={13} />
              计费即代码 · Billing as Code
            </span>
            <h1 className="text-4xl font-semibold tracking-tight text-foreground sm:text-5xl lg:text-[3.4rem] lg:leading-[1.1]">
              用量、套餐、账单，
              <br />
              <span className="text-brand-600">一次 API 全部搞定</span>
            </h1>
            <p className="max-w-lg text-base leading-relaxed text-muted-foreground">
              vLogBin 为云原生服务提供计量计费基础设施：用量上报、套餐目录、
              订阅与账单自动化。开发者只关心业务，计费交给 vLogBin。
            </p>
            <div className="flex flex-wrap items-center gap-3">
              <LinkButton href="/signup" size="lg">
                免费开始
              </LinkButton>
              <LinkButton href="/login" size="lg" variant="secondary">
                查看演示
              </LinkButton>
            </div>
            <ul className="flex flex-wrap gap-x-5 gap-y-2 text-sm text-muted-foreground">
              {["30 秒接入", "test / live 隔离", "OIDC 原生"].map((t) => (
                <li key={t} className="flex items-center gap-1.5">
                  <CheckIcon size={14} className="text-success" />
                  {t}
                </li>
              ))}
            </ul>
          </div>
          <TerminalHero />
        </div>
      </section>

      {/* 能力 */}
      <section id="features" className="border-t border-border bg-surface-1">
        <div className="mx-auto max-w-6xl px-6 py-20">
          <div className="mb-12 max-w-2xl">
            <h2 className="text-2xl font-semibold text-foreground sm:text-3xl">
              计量、计费、治理，一个平台
            </h2>
            <p className="mt-3 text-muted-foreground">
              从第一笔用量到年终账单，每个环节都可观测、可审计、可回滚。
            </p>
          </div>
          <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-4">
            {FEATURES.map((f) => (
              <div
                key={f.title}
                className="rounded-lg border border-border bg-surface-1 p-5 transition-shadow hover:shadow-md"
              >
                <span className="mb-3 inline-flex size-9 items-center justify-center rounded-md bg-brand-50 text-brand-600">
                  {f.icon}
                </span>
                <h3 className="text-sm font-semibold text-foreground">
                  {f.title}
                </h3>
                <p className="mt-1.5 text-sm leading-relaxed text-muted-foreground">
                  {f.description}
                </p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {/* 工作方式 */}
      <section id="how" className="mx-auto max-w-6xl px-6 py-20">
        <div className="mb-12 flex items-center gap-3">
          <span className="flex size-9 items-center justify-center rounded-md bg-terminal-bg text-terminal-fg">
            <BoxIcon size={17} />
          </span>
          <h2 className="text-2xl font-semibold text-foreground sm:text-3xl">
            三步接入
          </h2>
        </div>
        <div className="grid gap-6 sm:grid-cols-3">
          {STEPS.map((s) => (
            <div key={s.n} className="relative rounded-lg border border-border p-6">
              <span className="font-mono text-sm text-brand-600">{s.n}</span>
              <h3 className="mt-2 text-base font-semibold text-foreground">
                {s.title}
              </h3>
              <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
                {s.description}
              </p>
            </div>
          ))}
        </div>
      </section>

      {/* CTA */}
      <section id="cta" className="border-t border-border bg-terminal-bg">
        <div className="mx-auto flex max-w-6xl flex-col items-center gap-6 px-6 py-20 text-center">
          <GlobeIcon size={28} className="text-terminal-dim" />
          <h2 className="max-w-2xl text-2xl font-semibold text-terminal-fg sm:text-3xl">
            开始你的第一个计费闭环
          </h2>
          <p className="max-w-xl text-sm text-terminal-muted">
            无需信用卡。注册后即可获得隔离的 test 环境，30 秒完成首次用量上报。
          </p>
          <LinkButton href="/signup" size="lg" className="bg-terminal-fg text-terminal-bg hover:bg-terminal-fg/90">
            创建工作空间
          </LinkButton>
        </div>
      </section>

      {/* 页脚 */}
      <footer className="border-t border-border bg-surface-1">
        <div className="mx-auto flex max-w-6xl flex-col items-center justify-between gap-4 px-6 py-8 sm:flex-row">
          <Logo size={18} />
          <p className="text-xs text-muted-foreground">
            © {new Date().getFullYear()} vLogBin · Billing as Code
          </p>
        </div>
      </footer>
    </div>
  );
}
