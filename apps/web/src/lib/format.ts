const INVALID = "—";

function toDate(value?: string | null): Date | null {
  if (!value) return null;
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? null : d;
}

/** 短日期：2026年8月1日 */
export function formatDate(value?: string | null): string {
  const d = toDate(value);
  if (!d) return INVALID;
  return d.toLocaleDateString("zh-CN", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

/** 日期时间：2026年8月1日 14:30 */
export function formatDateTime(value?: string | null): string {
  const d = toDate(value);
  if (!d) return INVALID;
  return d.toLocaleString("zh-CN", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/** 金额：将分转换为货币字符串。 */
export function formatMoney(cents?: number | null, currency = "USD"): string {
  if (cents == null || Number.isNaN(cents)) return INVALID;
  return new Intl.NumberFormat("zh-CN", {
    style: "currency",
    currency,
    currencyDisplay: "narrowSymbol",
  }).format(cents / 100);
}

/** 数字千分位：1,234,567 */
export function formatNumber(value?: number | null): string {
  if (value == null || Number.isNaN(value)) return INVALID;
  return new Intl.NumberFormat("zh-CN").format(value);
}
