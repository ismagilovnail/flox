import { apiFetch } from "@/lib/api/client";

/** Mirrors apps/internal/onboarding: the Overview's "First steps",
 * each read from the organization's real data (audit 2026-10-15, UX-3). */
export type OnboardingStepId = "source" | "network" | "mapping" | "offer" | "campaign" | "click" | "conversion" | "cost";

export type OnboardingStep = {
  id: OnboardingStepId;
  done: boolean;
  /** The step could not be checked (clicks without ClickHouse). */
  unknown?: boolean;
};

export type OnboardingChecklist = { steps: OnboardingStep[]; done: number; total: number };

export function getOnboarding(): Promise<OnboardingChecklist> {
  return apiFetch("/onboarding");
}
