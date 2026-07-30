"use server";

import { revalidatePath } from "next/cache";
import {
  ApiError,
  createProvider,
  transitionLifecycle,
  type LifecycleTarget,
} from "@/lib/api";

export interface ActionState {
  ok: boolean;
  error?: string;
  apiKey?: string;
  providerId?: string;
}

export const initialActionState: ActionState = { ok: false };

function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message;
  if (err instanceof Error) return err.message;
  return "Unexpected error";
}

const LIFECYCLE_TARGETS: readonly LifecycleTarget[] = [
  "LIVE_REVIEW",
  "LIVE_ACTIVE",
  "RESTRICTED",
  "SUSPENDED",
];

export async function createProviderAction(
  _prev: ActionState,
  formData: FormData,
): Promise<ActionState> {
  const slug = String(formData.get("slug") ?? "").trim();
  const name = String(formData.get("name") ?? "").trim();
  const homeRegionCode = String(formData.get("home_region_code") ?? "").trim();

  if (!slug || !name || !homeRegionCode) {
    return { ok: false, error: "Slug, name and home region are required." };
  }

  try {
    const result = await createProvider({
      slug,
      name,
      home_region_code: homeRegionCode,
    });
    revalidatePath("/");
    return {
      ok: true,
      apiKey: result.apiKey ?? undefined,
      providerId: result.provider?.id || undefined,
    };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}

export async function lifecycleAction(
  _prev: ActionState,
  formData: FormData,
): Promise<ActionState> {
  const providerId = String(formData.get("provider_id") ?? "").trim();
  const to = String(formData.get("to") ?? "").trim() as LifecycleTarget;

  if (!providerId) {
    return { ok: false, error: "Missing provider id." };
  }
  if (!LIFECYCLE_TARGETS.includes(to)) {
    return { ok: false, error: `Invalid lifecycle target: ${to}` };
  }

  try {
    const result = await transitionLifecycle(providerId, to);
    revalidatePath("/");
    revalidatePath(`/providers/${providerId}`);
    return {
      ok: true,
      apiKey: result.apiKey ?? undefined,
      providerId,
    };
  } catch (err) {
    return { ok: false, error: errorMessage(err) };
  }
}
