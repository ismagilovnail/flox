"use client";

import * as React from "react";
import { SendIcon } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { usePermission } from "@/hooks/use-auth";
import { useOfflineConversions, useUpdateOfflineConversions } from "@/hooks/use-ad-account-connection";
import type { OfflineConversions, OfflineConversionsInput, OfflineSkipReason } from "@/lib/api/ad-account-connections";
import { ApiError } from "@/lib/api/client";

const ACTION_FIELDS = ["leadActionId", "purchaseActionId", "repurchaseActionId"] as const;
type Field = (typeof ACTION_FIELDS)[number] | "loginAccountId";

const SKIP_REASONS: OfflineSkipReason[] = ["no_google_click", "no_action", "not_attributed", "click_not_found"];

function toInput(s: OfflineConversions): OfflineConversionsInput {
  return {
    enabled: s.enabled,
    loginAccountId: s.loginAccountId,
    leadActionId: s.leadActionId,
    purchaseActionId: s.purchaseActionId,
    repurchaseActionId: s.repurchaseActionId,
  };
}

/** Under a connected Google Ads account: report the source's leads and
 * purchases back to Google by the click's gclid (audit UNI-2 part 2,
 * docs/offline-conversions.md). The worker sends; this shows what came of
 * it. No <form>: it sits inside SourceForm's own (see
 * AdAccountConnectionSection). */
export function OfflineConversionsSection({ trafficSourceId }: { trafficSourceId: string }) {
  const { t } = useTranslation("trafficSources");
  const canWrite = usePermission("source.write");
  const query = useOfflineConversions(trafficSourceId, true);
  const update = useUpdateOfflineConversions(trafficSourceId);
  const [draft, setDraft] = React.useState<OfflineConversionsInput | null>(null);
  const [errors, setErrors] = React.useState<Partial<Record<Field, string>>>({});

  const data = query.data;
  const values = draft ?? (data ? toInput(data) : null);
  if (query.isLoading) return <p className="text-xs text-muted-foreground">{t("offline.loading")}</p>;
  if (!data || !values) return null;

  function set<K extends keyof OfflineConversionsInput>(key: K, value: OfflineConversionsInput[K]) {
    setDraft({ ...values!, [key]: value });
  }

  function save() {
    update.mutate(values!, {
      onSuccess: () => {
        setDraft(null);
        setErrors({});
        toast(t("offline.saved"));
      },
      onError: (err) => {
        if (err instanceof ApiError && err.fields) setErrors(err.fields as Partial<Record<Field, string>>);
        toast.error(t("offline.saveError"), { description: err.message });
      },
    });
  }

  const counts = data.counts;
  const skipped = SKIP_REASONS.filter((r) => (data.skipped[r] ?? 0) > 0);

  return (
    <div className="flex flex-col gap-3 rounded-md border border-border p-2.5">
      <div className="flex items-start justify-between gap-3">
        <div className="grid gap-0.5">
          <span className="text-sm font-medium">{t("offline.title")}</span>
          <p className="max-w-prose text-xs text-muted-foreground">{t("offline.description")}</p>
        </div>
        <Switch
          aria-label={t("offline.enabledAria")}
          checked={values.enabled}
          onCheckedChange={(v) => set("enabled", v)}
          disabled={!canWrite}
        />
      </div>

      <div className="grid gap-2 sm:grid-cols-2">
        {ACTION_FIELDS.map((field) => (
          <div key={field} className="grid gap-1.5">
            <Label htmlFor={`offline-${field}`}>{t(`offline.fields.${field}`)}</Label>
            <Input
              id={`offline-${field}`}
              inputMode="numeric"
              className="font-mono text-xs"
              placeholder="123456789"
              value={values[field]}
              onChange={(e) => set(field, e.target.value)}
              aria-invalid={!!errors[field]}
              disabled={!canWrite}
            />
            {errors[field] && <p className="text-xs text-danger">{errors[field]}</p>}
          </div>
        ))}
        <div className="grid gap-1.5">
          <Label htmlFor="offline-loginAccountId">{t("offline.fields.loginAccountId")}</Label>
          <Input
            id="offline-loginAccountId"
            className="font-mono text-xs"
            placeholder="123-456-7890"
            value={values.loginAccountId}
            onChange={(e) => set("loginAccountId", e.target.value)}
            aria-invalid={!!errors.loginAccountId}
            disabled={!canWrite}
          />
          {errors.loginAccountId && <p className="text-xs text-danger">{errors.loginAccountId}</p>}
        </div>
      </div>
      <p className="text-xs text-muted-foreground">{t("offline.actionsHint")}</p>
      <p className="text-xs text-warning">{t("offline.scopeHint")}</p>

      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
          <span>{t("offline.counts.sent", { count: counts.sent ?? 0 })}</span>
          <span>{t("offline.counts.pending", { count: counts.pending ?? 0 })}</span>
          {(counts.failed ?? 0) > 0 && <span className="text-danger">{t("offline.counts.failed", { count: counts.failed })}</span>}
        </div>
        <Button type="button" size="sm" onClick={save} disabled={!canWrite || update.isPending || !draft}>
          <SendIcon className="size-3.5" /> {t("offline.save")}
        </Button>
      </div>
      {skipped.length > 0 && (
        <ul className="flex flex-col gap-0.5 text-xs text-muted-foreground">
          {skipped.map((r) => (
            <li key={r}>{t(`offline.skipped.${r}`, { count: data.skipped[r] })}</li>
          ))}
        </ul>
      )}
      {data.lastError && (
        <p className="text-xs break-words text-danger">
          {t("offline.lastError")} {data.lastError}
        </p>
      )}
    </div>
  );
}
