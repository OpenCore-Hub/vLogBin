export interface CustomHeader {
  key: string;
  value?: string;
}

export function parseCustomHeaders(raw?: string): CustomHeader[] {
  if (!raw) {
    return [];
  }
  const headers: CustomHeader[] = [];
  for (const entry of raw.split(",")) {
    const separator = entry.indexOf(":");
    if (separator <= 0) {
      continue;
    }
    const key = entry.slice(0, separator).trim();
    const value = entry.slice(separator + 1).trim();
    headers.push({ key, value: value || undefined });
  }
  return headers;
}

export function applyCustomHeaders(
  target: { set: (key: string, value: string) => void; delete: (key: string) => void },
  raw?: string,
): void {
  for (const header of parseCustomHeaders(raw)) {
    if (header.value) {
      target.set(header.key, header.value);
    } else {
      target.delete(header.key);
    }
  }
}
