"use client";

import { useTranslation } from "react-i18next";

import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { ErrorState } from "@/components/ui/error-state";
import { LoadingState } from "@/components/ui/loading-state";
import { useApiErrorMessage } from "@/hooks/use-api-error-message";
import { useBilling, usePlans } from "@/hooks/use-billing";
import { useLocale } from "@/hooks/use-locale";
import type { BillingLevel, Plan } from "@/lib/api/billing";
import { formatInt, formatMoney } from "@/lib/format";
import { cn } from "@/lib/utils";

const LEVEL_VARIANT: Record<BillingLevel, "secondary" | "warning" | "success" | "danger"> = {
  ok: "success",
  warning: "warning",
  exceeded: "danger",
  blocked: "danger",
  unknown: "secondary",
};

const BAR_COLOR: Record<BillingLevel, string> = {
  ok: "bg-success",
  warning: "bg-warning",
  exceeded: "bg-danger",
  blocked: "bg-danger",
  unknown: "bg-muted-foreground",
};

/** Settings › Plan (BIZ-1): the plan, this month's clicks against it, the
 * count limits and every plan. Plans are changed by the FLOX team for now
 * (apps/cmd/billing) — there is no checkout to link to yet. */
export function BillingPanel() {
  const { t } = useTranslation("settings");
  const locale = useLocale();
  const errorMessage = useApiErrorMessage();
  const billing = useBilling({ fresh: true });
  const plans = usePlans();

  if (billing.isPending) return <LoadingState label={t("billing.loading")} />;
  if (billing.isError) {
    return <ErrorState title={t("billing.loadError")} description={errorMessage(billing.error)} onRetry={() => billing.refetch()} />;
  }

  const { plan, usage, period, level } = billing.data;
  const limit = (n: number | null) => (n === null ? t("billing.unlimited") : formatInt(n, locale));
  const share = plan.monthlyClicks && usage.clicks !== null ? usage.clicks / plan.monthlyClicks : null;
  const counts: { key: "campaigns" | "domains" | "members"; used: number; max: number | null }[] = [
    { key: "campaigns", used: usage.campaigns, max: plan.maxCampaigns },
    { key: "domains", used: usage.domains, max: plan.maxDomains },
    { key: "members", used: usage.members, max: plan.maxMembers },
  ];

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle className="flex flex-wrap items-center gap-2">
            {t("billing.current", { plan: plan.name })}
            <Badge variant={LEVEL_VARIANT[level]}>{t(`billing.level.${level}`)}</Badge>
          </CardTitle>
          <CardDescription>{t("billing.description")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-5">
          <div className="flex flex-col gap-2">
            <div className="flex flex-wrap items-baseline justify-between gap-2 text-sm">
              <span className="font-medium">{t("billing.clicks", { from: period.from, to: period.to })}</span>
              <span className="tabular-nums">
                {usage.clicks === null ? "—" : formatInt(usage.clicks, locale)} / {limit(plan.monthlyClicks)}
              </span>
            </div>
            {share !== null && (
              <div
                className="h-2 overflow-hidden rounded-full bg-muted"
                role="progressbar"
                aria-valuemin={0}
                aria-valuemax={plan.monthlyClicks ?? 0}
                aria-valuenow={usage.clicks ?? 0}
                aria-label={t("billing.clicksAria")}
              >
                <div className={cn("h-full rounded-full", BAR_COLOR[level])} style={{ width: `${Math.min(share, 1) * 100}%` }} />
              </div>
            )}
            <p className="text-xs text-muted-foreground">{t(`billing.hint.${level}`)}</p>
          </div>

          <dl className="grid gap-3 sm:grid-cols-3">
            {counts.map((c) => (
              <div key={c.key} className="rounded-md border px-3 py-2">
                <dt className="text-xs text-muted-foreground">{t(`billing.counts.${c.key}`)}</dt>
                <dd className={cn("text-lg font-semibold tabular-nums", c.max !== null && c.used >= c.max && "text-warning")}>
                  {formatInt(c.used, locale)} / {limit(c.max)}
                </dd>
              </div>
            ))}
          </dl>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("billing.plansTitle")}</CardTitle>
          <CardDescription>{t("billing.plansDescription")}</CardDescription>
        </CardHeader>
        <CardContent>
          {plans.isPending ? (
            <LoadingState label={t("billing.loading")} />
          ) : plans.isError ? (
            <ErrorState title={t("billing.loadError")} description={errorMessage(plans.error)} onRetry={() => plans.refetch()} />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b text-left text-xs text-muted-foreground">
                    <th className="py-2 pr-4 font-medium">{t("billing.table.plan")}</th>
                    <th className="py-2 pr-4 text-right font-medium">{t("billing.table.price")}</th>
                    <th className="py-2 pr-4 text-right font-medium">{t("billing.table.clicks")}</th>
                    <th className="py-2 pr-4 text-right font-medium">{t("billing.counts.campaigns")}</th>
                    <th className="py-2 pr-4 text-right font-medium">{t("billing.counts.domains")}</th>
                    <th className="py-2 text-right font-medium">{t("billing.counts.members")}</th>
                  </tr>
                </thead>
                <tbody>
                  {plans.data.plans.map((p: Plan) => (
                    <tr key={p.code} className={cn("border-b last:border-0", p.code === plan.code && "bg-accent")}>
                      <td className="py-2 pr-4 font-medium">
                        {p.name}
                        {p.code === plan.code && <span className="ml-2 text-xs text-muted-foreground">{t("billing.yours")}</span>}
                      </td>
                      <td className="py-2 pr-4 text-right tabular-nums">
                        {p.priceCents === 0 ? t("billing.free") : t("billing.perMonth", { price: formatMoney(p.priceCents / 100, p.currency, 0, locale) })}
                      </td>
                      <td className="py-2 pr-4 text-right tabular-nums">{limit(p.monthlyClicks)}</td>
                      <td className="py-2 pr-4 text-right tabular-nums">{limit(p.maxCampaigns)}</td>
                      <td className="py-2 pr-4 text-right tabular-nums">{limit(p.maxDomains)}</td>
                      <td className="py-2 text-right tabular-nums">{limit(p.maxMembers)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
