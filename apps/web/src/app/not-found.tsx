import Link from "next/link";
import { Button } from "@/components/ui/button";
import { LogoMark } from "@/components/ui/icons";

export default function NotFound() {
  return (
    <main className="flex min-h-dvh items-center justify-center bg-canvas px-4">
      <div className="surface-premium w-full max-w-md rounded-2xl border border-border p-8 text-center">
        <LogoMark size={28} className="mx-auto" />
        <p className="mt-5 font-mono text-xs uppercase tracking-[0.2em] text-muted-foreground">
          404 · Not Found
        </p>
        <h1 className="mt-2 text-xl font-semibold tracking-tight">页面不存在</h1>
        <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
          你访问的地址不存在或已被移动。返回控制台继续工作。
        </p>
        <div className="mt-6 flex justify-center">
          <Link href="/console" prefetch={false}>
            <Button variant="primary">返回控制台</Button>
          </Link>
        </div>
      </div>
    </main>
  );
}
