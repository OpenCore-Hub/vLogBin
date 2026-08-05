"use client";

import {
  createContext,
  useCallback,
  useContext,
  useState,
  type ReactNode,
} from "react";
import { useToast } from "@/components/ui/toast";
import type { Env } from "@/lib/env-shared";
import { toErrorMessage } from "@/lib/utils";

type EnvContextValue = {
  env: Env;
  switchTo: (env: Env) => Promise<void>;
  pending: boolean;
};

const EnvContext = createContext<EnvContextValue | null>(null);

/**
 * 环境切换的客户端单一事实源。
 *
 * 服务端（cookie/URL）解析出的 env 作为 initialEnv 传入；客户端切换时
 * 乐观更新本地状态，让触发器/徽标立即反映目标环境，同时写 cookie
 * （onChange Server Action）并同步 ?env= URL。服务端重渲染或刷新后
 * initialEnv 变化会通过 useEffect 同步回来。
 *
 * 背景：R27 曾因受控组件 value 依赖服务端重渲染而不同步——切换后
 * 触发器仍显示旧环境。改为 context 后，顶栏徽标与切换器共享同一状态。
 */
export function EnvProvider({
  initialEnv,
  onChange,
  children,
}: {
  initialEnv: Env;
  onChange: (env: Env) => Promise<void>;
  children: ReactNode;
}) {
  const { toast } = useToast();
  const [env, setEnv] = useState<Env>(initialEnv);
  const [pending, setPending] = useState(false);

  // 服务端解析（cookie/URL）变化时同步本地状态；React 官方的
  // “render 期间调整状态”模式，避免 effect 内 setState 连锁渲染。
  if (env !== initialEnv) {
    setEnv(initialEnv);
  }

  const switchTo = useCallback(
    async (target: Env) => {
      if (target === env) return;
      const previous = env;
      setPending(true);
      setEnv(target); // 乐观更新：UI 立即反映目标环境。
      try {
        await onChange(target);
        // URL 显式 ?env=（临时覆盖，不写 cookie）由客户端组件追加。
        const params = new URLSearchParams(window.location.search);
        params.set("env", target);
        const qs = params.toString();
        window.history.replaceState(
          null,
          "",
          qs ? `${window.location.pathname}?${qs}` : window.location.pathname,
        );
        toast({
          variant: "success",
          title: target === "live" ? "已切换到生产环境" : "已切换到测试环境",
          description:
            target === "live"
              ? "当前页面将读取生产环境数据，操作请谨慎。"
              : "当前页面已回到隔离的沙箱环境。",
        });
      } catch (err) {
        setEnv(previous);
        toast({
          variant: "danger",
          title: "环境切换失败",
          description: toErrorMessage(err, "请稍后重试"),
        });
      } finally {
        setPending(false);
      }
    },
    [env, onChange, toast],
  );

  return (
    <EnvContext.Provider value={{ env, switchTo, pending }}>
      {children}
    </EnvContext.Provider>
  );
}

export function useEnv(): EnvContextValue {
  const ctx = useContext(EnvContext);
  if (!ctx) {
    throw new Error("useEnv must be used within <EnvProvider>");
  }
  return ctx;
}
