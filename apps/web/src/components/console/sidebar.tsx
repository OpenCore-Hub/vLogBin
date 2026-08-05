"use client";

import { useState, type SVGProps } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { cn } from "@/lib/utils";
import { LogoCompact } from "@/components/brand/logo";
import {
  ActivityIcon,
  AppIcon,
  BoxIcon,
  CreditCardIcon,
  HomeIcon,
  KeyIcon,
  MenuIcon,
  PackageIcon,
  SettingsIcon,
  UsersIcon,
  WebhookIcon,
  XIcon,
} from "@/components/ui/icons";

type NavItem = {
  href: string;
  label: string;
  icon: (p: SVGProps<SVGSVGElement> & { size?: number }) => React.ReactNode;
};

const NAV_GROUPS: Array<{ label: string; items: NavItem[] }> = [
  {
    label: "工作区",
    items: [
      { href: "/console", label: "概览", icon: HomeIcon },
      { href: "/ops", label: "运营商台", icon: BoxIcon },
      { href: "/console/settings", label: "设置", icon: SettingsIcon },
    ],
  },
  {
    label: "身份与访问",
    items: [
      {
        href: "/console/identity/applications",
        label: "应用",
        icon: AppIcon,
      },
    ],
  },
  {
    label: "计费",
    items: [
      {
        href: "/console/billing/plans",
        label: "套餐",
        icon: PackageIcon,
      },
      {
        href: "/console/billing/customers",
        label: "客户",
        icon: UsersIcon,
      },
      {
        href: "/console/billing/invoices",
        label: "账单",
        icon: CreditCardIcon,
      },
    ],
  },
  {
    label: "开发者",
    items: [
      {
        href: "/console/developers/api-keys",
        label: "API Keys",
        icon: KeyIcon,
      },
      {
        href: "/console/developers/webhooks",
        label: "Webhooks",
        icon: WebhookIcon,
      },
      {
        href: "/console/developers/events",
        label: "事件流",
        icon: ActivityIcon,
      },
    ],
  },
];

function NavLinks({ pathname }: { pathname: string }) {
  const isActive = (href: string) =>
    href === "/console"
      ? pathname === href || pathname.startsWith("/console/")
      : pathname === href || pathname.startsWith(`${href}/`);

  return (
    <>
      {NAV_GROUPS.map((group) => (
        <div key={group.label}>
          <p className="px-2 pb-2 text-[11px] font-semibold uppercase tracking-[0.18em] text-muted-foreground">
            {group.label}
          </p>
          <ul className="space-y-0.5">
            {group.items.map((item) => {
              const active = isActive(item.href);
              const Icon = item.icon;
              return (
                <li key={item.href}>
                  <Link
                    href={item.href}
                    prefetch={false}
                    aria-current={active ? "page" : undefined}
                    className={cn(
                      "pressable flex items-center gap-2.5 rounded-full px-3 py-2 text-sm font-medium",
                      active
                        ? "bg-brand-700 text-white shadow-[var(--shadow-premium)]"
                        : "text-muted-foreground hover:bg-surface-2 hover:text-foreground",
                    )}
                  >
                    <Icon size={16} />
                    {item.label}
                  </Link>
                </li>
              );
            })}
          </ul>
        </div>
      ))}
    </>
  );
}

/** 桌面侧边栏 + 窄屏抽屉（R21：<lg 折叠为汉堡入口，选择后自动收起）。 */
export function Sidebar() {
  const pathname = usePathname();
  const [drawerOpen, setDrawerOpen] = useState(false);

  return (
    <>
      {/* 桌面侧边栏 */}
      <aside className="fixed inset-y-0 left-0 z-30 hidden w-60 flex-col border-r border-border bg-surface-1/95 backdrop-blur-sm lg:flex">
        <div className="flex h-16 items-center border-b border-border px-5">
          <Link href="/console" aria-label="vLogBin 控制台">
            <LogoCompact />
          </Link>
        </div>
        <nav className="flex-1 space-y-6 overflow-y-auto px-3 py-5">
          <NavLinks pathname={pathname} />
        </nav>
      </aside>

      {/* 窄屏：汉堡按钮 */}
      <button
        type="button"
        aria-label="打开导航菜单"
        aria-expanded={drawerOpen}
        onClick={() => setDrawerOpen(true)}
        className="fixed left-3 top-3 z-30 inline-flex size-9 items-center justify-center rounded-md border border-border bg-surface-1 text-muted-foreground transition-colors hover:text-foreground lg:hidden"
      >
        <MenuIcon size={18} />
      </button>

      {/* 窄屏：抽屉 */}
      {drawerOpen && (
        <div
          className="fixed inset-0 z-40 lg:hidden"
          role="dialog"
          aria-modal="true"
          aria-label="导航菜单"
        >
          <div
            className="absolute inset-0 bg-neutral-950/45 animate-fade-in"
            onClick={() => setDrawerOpen(false)}
            aria-hidden="true"
          />
          <div className="absolute inset-y-0 left-0 flex w-64 flex-col border-r border-border bg-surface-1 shadow-lg animate-slide-up">
            <div className="flex h-16 items-center justify-between border-b border-border px-4">
              <Link
                href="/console"
                aria-label="vLogBin 控制台"
                onClick={() => setDrawerOpen(false)}
              >
                <LogoCompact />
              </Link>
              <button
                type="button"
                aria-label="关闭导航菜单"
                onClick={() => setDrawerOpen(false)}
                className="rounded-md p-1 text-muted-foreground transition-colors hover:bg-surface-2 hover:text-foreground"
              >
                <XIcon size={18} />
              </button>
            </div>
            <nav
              className="flex-1 space-y-6 overflow-y-auto px-3 py-5"
              onClick={() => setDrawerOpen(false)}
            >
              <NavLinks pathname={pathname} />
            </nav>
          </div>
        </div>
      )}
    </>
  );
}
