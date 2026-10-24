"use client";

import * as React from "react";
import { SendIcon, Unlink2Icon } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { usePermission } from "@/hooks/use-auth";
import { useDeleteMetricaConnection, useMetricaConnection, useUpdateMetricaConnection } from "@/hooks/use-ad-account-connection";
import type { MetricaConnection, MetricaInput, MetricaSkipReason } from "@/lib/api/ad-account-connections";
import { ApiError } from "@/lib/api/client";
import { CURRENCIES } from "@/lib/countries";

const GOAL_FIELDS = ["leadGoal", "purchaseGoal", "repurchaseGoal"] as const;
type Field = (typeof GOAL_FIELDS)[number] | "counterId" | "token" | "currency";

const SKIP_REASONS: MetricaSkipReason[] = ["no_yandex_click", "visit_too_old", "no_action", "not_attributed", "click_not_found"];

function toInput(s: MetricaConnection): MetricaInput {
  return {
    enabled: s.enabled,
    counterId: s.counterId,
    token: "",
    currency: s.currency || "RUB",
    leadGoal: s.leadGoal,
    purchaseGoal: s.purchaseGoal,
    repurchaseGoal: s.repurchaseGoal,
  };
}

/** Any source whose clicks come from Yandex Direct with yclid: report its
 * leads and purchases to the Metrica counter on the landing (audit UNI-2
 * part 3, docs/offline-conversions.md). The worker uploads; this shows
 * what came of it. No <form>: it sits inside SourceForm's own (see
 * AdAccountConnectionSection). */
export function MetricaSection({ trafficSourceId }: { trafficSourceId: string }) {
  const { t } = useTranslation("trafficSources");
  const canWrite = usePermission("source.write");
  const query = useMetricaConnection(trafficSourceId);
  const update = useUpdateMetricaConnection(trafficSourceId);
  const remove = useDeleteMetricaConnection(trafficSourceId);
  const [draft, setDraft] = React.useState<MetricaInput | null>(null);
  const [errors, setErrors] = React.useState<Partial<Record<Field, string>>>({});
  const [confirmingDelete, setConfirmingDelete] = React.useState(false);

  const data = query.data;
  const values = draft ?? (data ? toInput(data) : null);
  if (query.isLoading) return <p className="text-xs text-muted-foreground">{t("metrica.loading")}</p>;
  if (query.isError)
    return (
      <p className="text-xs text-danger">
        {t("metrica.loadError")} {query.error.message}
      </p>
    );
  if (!data || !values) return null;

  function set<K extends keyof MetricaInput>(key: K, value: MetricaInput[K]) {
    setDraft({ ...values!, [key]: value });
  }

  function save() {
    update.mutate(values!, {
      onSuccess: () => {
        setDraft(null);
        setErrors({});
        toast(t("metrica.saved"));
      },
      onError: (err) => {
        if (err instanceof ApiError && err.fields) setErrors(err.fields as Partial<Record<Field, string>>);
        toast.error(t("metrica.saveError"), { description: err.message });
      },
    });
  }

  function disconnect() {
    remove.mutate(undefined, {
      onSuccess: () => {
        setDraft(null);
        setErrors({});
        setConfirmingDelete(false);
        toast(t("metrica.disconnected"));
      },
      onError: (err) => toast.error(t("metrica.disconnectError"), { description: err.message }),
    });
  }

  const counts = data.counts;
  const skipped = SKIP_REASONS.filter((r) => (data.skipped[r] ?? 0) > 0);
  const canSave = canWrite && !update.isPending && !!draft && (data.connected || values.token.trim() !== "");

  return (
    <div className="flex flex-col gap-3 rounded-lg border border-border p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="grid gap-0.5">
          <span className="text-sm font-medium">{t("metrica.title")}</span>
          <p className="max-w-prose text-xs text-muted-foreground">{t("metrica.description")}</p>
        </div>
        {data.connected && (
          <Switch
            aria-label={t("metrica.enabledAria")}
            checked={values.enabled}
            onCheckedChange={(v) => set("enabled", v)}
            disabled={!canWrite}
          />
        )}
      </div>

      <div className="grid gap-2 sm:grid-cols-2">
        <div className="grid gap-1.5">
          <Label htmlFor="metrica-counterId">{t("metrica.fields.counterId")}</Label>
          <Input
            id="metrica-counterId"
            inputMode="numeric"
            className="font-mono text-xs"
            placeholder="12345678"
            value={values.counterId}
            onChange={(e) => set("counterId", e.target.value)}
            aria-invalid={!!errors.counterId}
            disabled={!canWrite}
          />
          {errors.counterId && <p className="text-xs text-danger">{errors.counterId}</p>}
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="metrica-token">{t("metrica.fields.token")}</Label>
          <Input
            id="metrica-token"
            type="password"
            autoComplete="off"
            className="font-mono text-xs"
            placeholder={data.connected ? t("metrica.tokenKeep", { last4: data.tokenPreview }) : "y0_…"}
            value={values.token}
            onChange={(e) => set("token", e.target.value)}
            aria-invalid={!!errors.token}
            disabled={!canWrite}
          />
          {errors.token && <p className="text-xs text-danger">{errors.token}</p>}
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="metrica-currency">{t("metrica.fields.currency")}</Label>
          <Select value={values.currency} onValueChange={(v) => set("currency", v)} disabled={!canWrite}>
            <SelectTrigger id="metrica-currency" className="w-full" aria-invalid={!!errors.currency}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {CURRENCIES.map((c) => (
                <SelectItem key={c} value={c}>
                  {c}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {errors.currency && <p className="text-xs text-danger">{errors.currency}</p>}
        </div>
        {GOAL_FIELDS.map((field) => (
          <div key={field} className="grid gap-1.5">
            <Label htmlFor={`metrica-${field}`}>{t(`metrica.fields.${field}`)}</Label>
            <Input
              id={`metrica-${field}`}
              className="font-mono text-xs"
              placeholder="order_confirmed"
              value={values[field]}
              onChange={(e) => set(field, e.target.value)}
              aria-invalid={!!errors[field]}
              disabled={!canWrite}
            />
            {errors[field] && <p className="text-xs text-danger">{errors[field]}</p>}
          </div>
        ))}
      </div>
      <p className="text-xs text-muted-foreground">{t("metrica.goalsHint")}</p>
      <p className="text-xs text-warning">{t("metrica.setupHint")}</p>

      <div className="flex flex-wrap items-center justify-between gap-2">
        {data.connected ? (
          <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
            <span>{t("metrica.counts.sent", { count: counts.sent ?? 0 })}</span>
            {data.processing > 0 && <span>{t("metrica.counts.processing", { count: data.processing })}</span>}
            <span>{t("metrica.counts.pending", { count: counts.pending ?? 0 })}</span>
            {(counts.failed ?? 0) > 0 && <span className="text-danger">{t("metrica.counts.failed", { count: counts.failed })}</span>}
          </div>
        ) : (
          <span />
        )}
        <div className="flex items-center gap-2">
          {data.connected &&
            (confirmingDelete ? (
              <>
                <Button type="button" variant="outline" size="sm" onClick={() => setConfirmingDelete(false)}>
                  {t("metrica.cancel")}
                </Button>
                <Button type="button" variant="destructive" size="sm" onClick={disconnect} disabled={!canWrite || remove.isPending}>
                  {t("metrica.confirmDisconnect")}
                </Button>
              </>
            ) : (
              <Button type="button" variant="outline" size="sm" onClick={() => setConfirmingDelete(true)} disabled={!canWrite}>
                <Unlink2Icon className="size-3.5" /> {t("metrica.disconnect")}
              </Button>
            ))}
          <Button type="button" size="sm" onClick={save} disabled={!canSave}>
            <SendIcon className="size-3.5" /> {data.connected ? t("metrica.save") : t("metrica.connect")}
          </Button>
        </div>
      </div>
      {skipped.length > 0 && (
        <ul className="flex flex-col gap-0.5 text-xs text-muted-foreground">
          {skipped.map((r) => (
            <li key={r}>{t(`metrica.skipped.${r}`, { count: data.skipped[r] })}</li>
          ))}
        </ul>
      )}
      {data.lastError && (
        <p className="text-xs break-words text-danger">
          {t("metrica.lastError")} {data.lastError}
        </p>
      )}
    </div>
  );
}
