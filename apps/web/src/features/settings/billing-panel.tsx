"use client";

import * as React from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, Loader2Icon, XCircle } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { ErrorState } from "@/components/ui/error-state";
import { Label } from "@/components/ui/label";
import { LoadingState } from "@/components/ui/loading-state";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { useApiErrorMessage } from "@/hooks/use-api-error-message";
import { useMe, usePermission } from "@/hooks/use-auth";
import { billingKey, useBilling, useCheckout, usePayment, usePayments, usePlans } from "@/hooks/use-billing";
import { useLocale } from "@/hooks/use-locale";
import type { BillingLevel, Payment, PaymentMonths, Plan } from "@/lib/api/billing";
import { formatDeletionDate, formatInt, formatMoney } from "@/lib/format";
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

const PAYMENT_VARIANT: Record<Payment["status"], "warning" | "success" | "secondary"> = {
  pending: "warning",
  succeeded: "success",
  canceled: "secondary",
};

const MONTHS: PaymentMonths[] = [1, 3, 12];

/** Settings › Plan (BIZ-1): the plan and its paid period, this month's
 * clicks against it, the count limits, every plan and — for an owner, when
 * YooKassa is set up — paying for one (docs/billing.md "Payments"). */
export function BillingPanel() {
  const { t, i18n } = useTranslation("settings");
  const locale = useLocale();
  const errorMessage = useApiErrorMessage();
  const billing = useBilling({ fresh: true });
  const plans = usePlans();
  const me = useMe();
  const canManage = usePermission("settings.write");
  const [paying, setPaying] = React.useState<Plan | null>(null);

  if (billing.isPending) return <LoadingState label={t("billing.loading")} />;
  if (billing.isError) {
    return <ErrorState title={t("billing.loadError")} description={errorMessage(billing.error)} onRetry={() => billing.refetch()} />;
  }

  const { plan, usage, period, level } = billing.data;
  const isOwner = me.data?.role === "Owner";
  const canPay = billing.data.canPay && isOwner;
  const limit = (n: number | null) => (n === null ? t("billing.unlimited") : formatInt(n, locale));
  const rub = (kopecks: number) => formatMoney(kopecks / 100, "RUB", 0, locale);
  const share = plan.monthlyClicks && usage.clicks !== null ? usage.clicks / plan.monthlyClicks : null;
  const counts: { key: "campaigns" | "domains" | "members"; used: number; max: number | null }[] = [
    { key: "campaigns", used: usage.campaigns, max: plan.maxCampaigns },
    { key: "domains", used: usage.domains, max: plan.maxDomains },
    { key: "members", used: usage.members, max: plan.maxMembers },
  ];

  return (
    <div className="flex flex-col gap-4">
      <PaymentReturnNotice />

      <Card>
        <CardHeader>
          <CardTitle className="flex flex-wrap items-center gap-2">
            {t("billing.current", { plan: plan.name })}
            <Badge variant={LEVEL_VARIANT[level]}>{t(`billing.level.${level}`)}</Badge>
          </CardTitle>
          <CardDescription>{t("billing.description")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-5">
          {billing.data.paidUntil && (
            <div className={cn("flex flex-wrap items-center gap-3 text-sm", billing.data.renew && "text-warning")}>
              <span>
                {t("billing.paidUntil", { date: formatDeletionDate(billing.data.paidUntil, i18n.language) })}
                {billing.data.renew && billing.data.graceUntil && (
                  <> — {t("billing.graceUntil", { date: formatDeletionDate(billing.data.graceUntil, i18n.language) })}</>
                )}
              </span>
              {canPay && plan.priceRub !== null && (
                <Button size="sm" variant={billing.data.renew ? "default" : "outline"} className="h-7" onClick={() => setPaying(plan)}>
                  {t("billing.pay.renew")}
                </Button>
              )}
            </div>
          )}

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
          <CardDescription>
            {canPay ? t("billing.plansDescriptionPay") : billing.data.canPay ? t("billing.plansDescriptionOwner") : t("billing.plansDescription")}
          </CardDescription>
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
                    <th className="py-2 pr-4 text-right font-medium">{t("billing.counts.members")}</th>
                    {canPay && <th className="py-2 font-medium" />}
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
                        {p.priceRub !== null
                          ? t("billing.perMonth", { price: rub(p.priceRub) })
                          : p.priceCents === 0
                            ? t("billing.free")
                            : t("billing.perMonth", { price: formatMoney(p.priceCents / 100, p.currency, 0, locale) })}
                      </td>
                      <td className="py-2 pr-4 text-right tabular-nums">{limit(p.monthlyClicks)}</td>
                      <td className="py-2 pr-4 text-right tabular-nums">{limit(p.maxCampaigns)}</td>
                      <td className="py-2 pr-4 text-right tabular-nums">{limit(p.maxDomains)}</td>
                      <td className="py-2 pr-4 text-right tabular-nums">{limit(p.maxMembers)}</td>
                      {canPay && (
                        <td className="py-2 text-right">
                          {p.priceRub !== null && (
                            <Button size="sm" variant="outline" className="h-7" onClick={() => setPaying(p)}>
                              {p.code === plan.code ? t("billing.pay.renew") : t("billing.pay.choose")}
                            </Button>
                          )}
                        </td>
                      )}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>

      {canManage && <PaymentHistory names={Object.fromEntries((plans.data?.plans ?? []).map((p) => [p.code, p.name]))} />}
      {paying && (
        <CheckoutDialog plan={paying} current={plan} carries={!!billing.data.paidUntil && plan.priceRub !== null} onClose={() => setPaying(null)} />
      )}
    </div>
  );
}

/** Choose 1, 3 or 12 months and go to YooKassa's payment page. */
/** carries: the current plan has paid days left to carry over to another. */
function CheckoutDialog({ plan, current, carries, onClose }: { plan: Plan; current: Plan; carries: boolean; onClose: () => void }) {
  const { t } = useTranslation("settings");
  const locale = useLocale();
  const errorMessage = useApiErrorMessage();
  const checkout = useCheckout();
  const [months, setMonths] = React.useState<PaymentMonths>(1);
  const price = plan.priceRub ?? 0;
  const rub = (kopecks: number) => formatMoney(kopecks / 100, "RUB", 0, locale);

  return (
    <Dialog open onOpenChange={(open) => !open && !checkout.isPending && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("billing.pay.title", { plan: plan.name })}</DialogTitle>
          <DialogDescription>
            {plan.code === current.code
              ? t("billing.pay.extends")
              : carries
                ? t("billing.pay.switchesCarry", { plan: current.name })
                : t("billing.pay.switches")}
          </DialogDescription>
        </DialogHeader>
        <form
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            checkout.mutate(
              { plan: plan.code, months },
              {
                onSuccess: (p) => {
                  if (p.confirmationUrl) window.location.assign(p.confirmationUrl);
                },
                onError: (err) => toast.error(t("billing.pay.error"), { description: errorMessage(err) }),
              },
            );
          }}
        >
          <RadioGroup value={String(months)} onValueChange={(v) => setMonths(Number(v) as PaymentMonths)} className="grid gap-2">
            {MONTHS.map((m) => (
              <Label key={m} htmlFor={`months-${m}`} className="flex cursor-pointer items-center justify-between gap-3 rounded-md border px-3 py-2 font-normal">
                <span className="flex items-center gap-2">
                  <RadioGroupItem id={`months-${m}`} value={String(m)} />
                  {t("billing.pay.months", { count: m })}
                </span>
                <span className="tabular-nums">{rub(price * m)}</span>
              </Label>
            ))}
          </RadioGroup>
          <p className="text-xs text-muted-foreground">{t("billing.pay.hint")}</p>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose} disabled={checkout.isPending}>
              {t("billing.pay.cancel")}
            </Button>
            <Button type="submit" disabled={checkout.isPending}>
              {checkout.isPending && <Loader2Icon className="size-4 animate-spin" />}
              {t("billing.pay.submit", { amount: rub(price * months) })}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/** Back from YooKassa (?payment=ID): the payment's outcome, read until
 * YooKassa settles it; on success the plan is read again. */
function PaymentReturnNotice() {
  const { t } = useTranslation("settings");
  const searchParams = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const qc = useQueryClient();
  const id = searchParams.get("payment");
  const payment = usePayment(id);
  const status = payment.data?.status;

  // Settled: the history (and, when paid, the plan) is read again.
  React.useEffect(() => {
    if (status === "succeeded" || status === "canceled") void qc.invalidateQueries({ queryKey: billingKey });
  }, [status, qc]);

  if (!id) return null;
  function dismiss() {
    const params = new URLSearchParams(searchParams.toString());
    params.delete("payment");
    router.replace(`${pathname}?${params.toString()}`, { scroll: false });
  }

  let icon = <Loader2Icon className="size-4 animate-spin text-muted-foreground" />;
  let text = t("billing.return.pending");
  if (payment.isError) {
    icon = <XCircle className="size-4 text-danger" />;
    text = t("billing.return.error");
  } else if (status === "succeeded") {
    icon = <CheckCircle2 className="size-4 text-success" />;
    text = t("billing.return.succeeded");
  } else if (status === "canceled") {
    icon = <XCircle className="size-4 text-danger" />;
    text = t("billing.return.canceled");
  }

  return (
    <div role="status" className="flex flex-wrap items-center gap-3 rounded-lg border px-4 py-3 text-sm">
      {icon}
      <span>{text}</span>
      {status === "pending" && payment.data?.confirmationUrl && (
        <Button asChild size="sm" variant="outline" className="h-7">
          <a href={payment.data.confirmationUrl}>{t("billing.return.continue")}</a>
        </Button>
      )}
      <Button size="sm" variant="ghost" className="ml-auto h-7" onClick={dismiss}>
        {t("billing.return.dismiss")}
      </Button>
    </div>
  );
}

function PaymentHistory({ names }: { names: Record<string, string> }) {
  const { t, i18n } = useTranslation("settings");
  const locale = useLocale();
  const errorMessage = useApiErrorMessage();
  const payments = usePayments(true);

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("billing.history.title")}</CardTitle>
      </CardHeader>
      <CardContent>
        {payments.isPending ? (
          <LoadingState label={t("billing.loading")} />
        ) : payments.isError ? (
          <ErrorState title={t("billing.history.loadError")} description={errorMessage(payments.error)} onRetry={() => payments.refetch()} />
        ) : payments.data.payments.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("billing.history.empty")}</p>
        ) : (
          <ul className="flex flex-col gap-2">
            {payments.data.payments.map((p) => (
              <li key={p.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
                <Badge variant={PAYMENT_VARIANT[p.status]}>{t(`billing.history.status.${p.status}`)}</Badge>
                <span className="text-muted-foreground">{formatDeletionDate(p.createdAt, i18n.language)}</span>
                <span className="font-medium">
                  {names[p.planCode] ?? p.planCode} · {t("billing.pay.months", { count: p.months })}
                </span>
                <span className="tabular-nums">{formatMoney(p.amount / 100, p.currency, 0, locale)}</span>
                {p.status === "canceled" && p.cancellationReason && (
                  <span className="text-xs text-muted-foreground">{p.cancellationReason}</span>
                )}
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
