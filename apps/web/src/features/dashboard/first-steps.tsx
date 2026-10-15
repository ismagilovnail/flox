"use client";

import * as React from "react";
import Link from "next/link";
import { CheckCircle2, Circle, CircleHelp, X } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { IconButton } from "@/components/ui/icon-button";
import { useOnboarding } from "@/hooks/use-onboarding";
import { useOrganization } from "@/hooks/use-organization";
import type { OnboardingStepId } from "@/lib/api/onboarding";
import { cn } from "@/lib/utils";

/** Where each step is done. */
const STEP_HREF: Record<OnboardingStepId, string> = {
  source: "/traffic-sources/new",
  network: "/networks/new",
  mapping: "/postbacks",
  offer: "/offers/new",
  campaign: "/campaigns/new",
  click: "/campaigns",
  conversion: "/networks",
  cost: "/campaigns",
};

const dismissKey = (orgId: string) => `flox-first-steps-dismissed:${orgId}`;

function readDismissed(orgId: string | undefined) {
  if (!orgId) return false;
  try {
    return localStorage.getItem(dismissKey(orgId)) === "1";
  } catch {
    return false;
  }
}

/** The Overview's "First steps": what is left before the first conversion
 * shows up here, each step ticked from the organization's real data (audit
 * 2026-10-15, UX-3). Gone once everything is done, or when hidden — per
 * organization, in this browser. */
export function FirstSteps() {
  const { t } = useTranslation("dashboard");
  const query = useOnboarding();
  const orgId = useOrganization().data?.id;
  const [hiddenNow, setHiddenNow] = React.useState(false);
  // Read at render: nothing renders before the checklist has loaded, which
  // never happens on the server, so this cannot cause a hydration mismatch.
  const dismissed = hiddenNow || readDismissed(orgId);

  // Secondary to the Overview: while loading or on an error it stays out of
  // the way instead of adding a second spinner or error box.
  if (!query.data || dismissed) return null;
  const { steps, done, total } = query.data;
  if (done === total) return null;
  const next = steps.find((s) => !s.done)?.id;

  function dismiss() {
    try {
      if (orgId) localStorage.setItem(dismissKey(orgId), "1");
    } catch {
      // Storage refused (private window): hidden for this visit only.
    }
    setHiddenNow(true);
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-start justify-between gap-3">
        <div className="grid gap-1">
          <CardTitle>{t("firstSteps.title", { done, total })}</CardTitle>
          <CardDescription>{t("firstSteps.description")}</CardDescription>
        </div>
        <IconButton aria-label={t("firstSteps.dismiss")} size="icon-sm" onClick={dismiss}>
          <X className="size-4" />
        </IconButton>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <div
          className="h-1.5 overflow-hidden rounded-full bg-muted"
          role="progressbar"
          aria-valuemin={0}
          aria-valuemax={total}
          aria-valuenow={done}
          aria-label={t("firstSteps.progressAria", { done, total })}
        >
          <div className="h-full rounded-full bg-success" style={{ width: `${(done / total) * 100}%` }} />
        </div>
        <ol className="grid gap-1 sm:grid-cols-2">
          {steps.map((s) => {
            const isNext = s.id === next;
            const Icon = s.done ? CheckCircle2 : s.unknown ? CircleHelp : Circle;
            return (
              <li
                key={s.id}
                className={cn(
                  "flex items-start gap-2.5 rounded-md px-2 py-2",
                  isNext && "bg-accent",
                )}
              >
                <Icon
                  aria-hidden="true"
                  className={cn("mt-0.5 size-4 shrink-0", s.done ? "text-success" : "text-muted-foreground")}
                />
                <div className="min-w-0 flex-1">
                  <p className={cn("text-sm font-medium", s.done && "text-muted-foreground line-through")}>
                    {t(`firstSteps.steps.${s.id}.title`)}
                    <span className="sr-only">
                      {" — "}
                      {s.done ? t("firstSteps.doneSr") : s.unknown ? t("firstSteps.unknownSr") : t("firstSteps.todoSr")}
                    </span>
                  </p>
                  {!s.done && (
                    <p className="text-xs text-muted-foreground">
                      {s.unknown ? t("firstSteps.unknown") : t(`firstSteps.steps.${s.id}.hint`)}
                    </p>
                  )}
                </div>
                {!s.done && (
                  <Button asChild size="sm" variant={isNext ? "default" : "ghost"} className="shrink-0">
                    <Link href={STEP_HREF[s.id]}>{t(`firstSteps.steps.${s.id}.action`)}</Link>
                  </Button>
                )}
              </li>
            );
          })}
        </ol>
      </CardContent>
    </Card>
  );
}
