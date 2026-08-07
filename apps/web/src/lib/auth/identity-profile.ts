export interface SplitDisplayName {
  givenName?: string;
  familyName?: string;
}

export function splitDisplayName(displayName?: string): SplitDisplayName {
  const parts = (displayName ?? "").trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return {};
  if (parts.length === 1) return { givenName: parts[0] };
  return { givenName: parts[0], familyName: parts.slice(1).join(" ") };
}
