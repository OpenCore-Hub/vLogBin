import { cn } from "@/lib/utils";
import { LogoMark } from "@/components/ui/icons";

/** 品牌组合标识：V 形 mark + wordmark。 */
export function Logo({
  size = 24,
  className,
  showWordmark = true,
}: {
  size?: number;
  className?: string;
  showWordmark?: boolean;
}) {
  return (
    <span className={cn("inline-flex items-center gap-2", className)}>
      <LogoMark size={size} className="text-brand-600" />
      {showWordmark && (
        <span className="font-mono text-lg font-semibold tracking-tight text-foreground">
          vLogBin
        </span>
      )}
    </span>
  );
}

/** 浅底 / 深底均可用的紧凑标识（用于顶部导航等）。 */
export function LogoCompact({ size = 20 }: { size?: number }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <LogoMark size={size} className="text-brand-600" />
      <span className="font-mono text-[15px] font-semibold text-foreground">
        vLogBin
      </span>
    </span>
  );
}
