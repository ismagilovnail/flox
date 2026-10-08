"use client";

import * as React from "react";
import { Controller, useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { PencilIcon, PlusIcon, Trash2Icon } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { z } from "zod";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Caption } from "@/components/ui/typography";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { usePermission } from "@/hooks/use-auth";
import { useAlertRules, useDeleteAlertRule, useSaveAlertRule } from "@/hooks/use-alerts";
import { useCampaigns } from "@/hooks/use-campaigns";
import { useIntegrations } from "@/hooks/use-integrations";
import { ALERT_KINDS, kindUsesCampaign, kindUsesThreshold, type AlertKind, type AlertRule, type AlertRuleInput } from "@/lib/alerts";

const ALL_CAMPAIGNS = "__all__";

/** Mirrors alert.Service.validate on the server, so the form says what is
 * wrong before a round trip; the server stays the authority. */
function ruleSchema(t: TFunction<"alerts">) {
  const optionalNumber = z
    .string()
    .trim()
    .refine((v) => v === "" || Number.isFinite(Number(v)), t("form.errors.thresholdRequired"));
  return z
    .object({
      kind: z.enum(ALERT_KINDS),
      campaignId: z.string(),
      threshold: optionalNumber,
      minSpend: optionalNumber,
      notifySlack: z.boolean(),
      enabled: z.boolean(),
    })
    .superRefine((v, ctx) => {
      const issue = (path: "threshold" | "minSpend", message: string) => ctx.addIssue({ code: "custom", path: [path], message });
      if (!kindUsesThreshold(v.kind)) return;
      if (v.threshold === "") return issue("threshold", t("form.errors.thresholdRequired"));
      const n = Number(v.threshold);
      if (v.kind === "spend_over" && n <= 0) issue("threshold", t("form.errors.positive"));
      if (v.kind === "roi_below" && (n < -100 || n > 100000)) issue("threshold", t("form.errors.roiRange"));
      if (v.kind === "ssl_expiring" && (!Number.isInteger(n) || n < 1 || n > 90)) issue("threshold", t("form.errors.days"));
      if (v.kind === "roi_below" && v.minSpend !== "" && Number(v.minSpend) < 0) issue("minSpend", t("form.errors.minSpend"));
    });
}

type RuleValues = z.infer<ReturnType<typeof ruleSchema>>;

function toValues(rule?: AlertRule): RuleValues {
  return {
    kind: rule?.kind ?? "spend_over",
    campaignId: rule?.campaignId ?? ALL_CAMPAIGNS,
    threshold: rule?.threshold != null ? String(rule.threshold) : "",
    minSpend: rule?.minSpend != null ? String(rule.minSpend) : "",
    notifySlack: rule?.notifySlack ?? false,
    enabled: rule?.enabled ?? true,
  };
}

function toInput(v: RuleValues): AlertRuleInput {
  return {
    kind: v.kind,
    campaignId: kindUsesCampaign(v.kind) && v.campaignId !== ALL_CAMPAIGNS ? v.campaignId : null,
    threshold: kindUsesThreshold(v.kind) && v.threshold !== "" ? Number(v.threshold) : null,
    minSpend: v.kind === "roi_below" && v.minSpend !== "" ? Number(v.minSpend) : null,
    notifySlack: v.notifySlack,
    enabled: v.enabled,
  };
}

function ruleInput(rule: AlertRule): AlertRuleInput {
  const { kind, campaignId, threshold, minSpend, notifySlack, enabled } = rule;
  return { kind, campaignId, threshold, minSpend, notifySlack, enabled };
}

export function AlertsPanel() {
  const { t } = useTranslation(["alerts", "common"]);
  const canWrite = usePermission("settings.write");
  const rulesQuery = useAlertRules();
  const campaignsQuery = useCampaigns();
  const integrationsQuery = useIntegrations();
  const save = useSaveAlertRule();
  const remove = useDeleteAlertRule();

  const [editing, setEditing] = React.useState<AlertRule | "new" | null>(null);
  const [deleting, setDeleting] = React.useState<AlertRule | null>(null);

  const campaigns = campaignsQuery.data?.campaigns ?? [];
  const campaignName = (id: string | null) =>
    id === null ? t("panel.allCampaigns") : (campaigns.find((c) => c.id === id)?.name ?? t("panel.unknownCampaign"));
  const slackConnected = integrationsQuery.data?.integrations.some((i) => i.provider === "slack" && i.status === "connected");
  const rules = rulesQuery.data?.rules ?? [];

  function summary(rule: AlertRule): string {
    const parts: string[] = [];
    if (kindUsesCampaign(rule.kind)) parts.push(campaignName(rule.campaignId));
    if (rule.kind === "spend_over") parts.push(t("summary.spend_over", { value: rule.threshold }));
    if (rule.kind === "roi_below")
      parts.push(
        rule.minSpend != null
          ? t("summary.roi_below_min", { value: rule.threshold, minSpend: rule.minSpend })
          : t("summary.roi_below", { value: rule.threshold }),
      );
    if (rule.kind === "ssl_expiring") parts.push(t("summary.ssl_expiring", { count: rule.threshold ?? 0 }));
    return parts.join(" · ");
  }

  function toggle(rule: AlertRule, enabled: boolean) {
    save.mutate(
      { id: rule.id, input: { ...ruleInput(rule), enabled } },
      { onError: (err) => toast.error(t("form.saveError"), { description: err.message }) },
    );
  }

  function confirmDelete() {
    if (!deleting) return;
    const name = t(`kinds.${deleting.kind}`);
    remove.mutate(deleting.id, {
      onSuccess: () => toast(t("delete.done"), { description: name }),
      onError: (err) => toast.error(t("delete.error"), { description: err.message }),
    });
    setDeleting(null);
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("panel.title")}</CardTitle>
        <CardDescription>{t("panel.description")}</CardDescription>
        {canWrite && (
          <CardAction>
            <Button size="sm" onClick={() => setEditing("new")}>
              <PlusIcon className="size-4" /> {t("panel.add")}
            </Button>
          </CardAction>
        )}
      </CardHeader>
      <CardContent className="flex flex-col divide-y divide-border p-0">
        {integrationsQuery.isSuccess && !slackConnected && rules.some((r) => r.notifySlack) && (
          <p className="px-4 py-3 text-xs text-warning">{t("panel.slackNotConnected")}</p>
        )}
        {rulesQuery.isPending && (
          <div className="px-4 py-6">
            <LoadingState label={t("panel.loading")} />
          </div>
        )}
        {rulesQuery.isError && (
          <div className="px-4 py-6">
            <ErrorState title={t("panel.loadError")} description={rulesQuery.error.message} onRetry={() => rulesQuery.refetch()} />
          </div>
        )}
        {rulesQuery.isSuccess && rules.length === 0 && <p className="px-4 py-6 text-sm text-muted-foreground">{t("panel.empty")}</p>}
        {rules.map((rule) => {
          const name = t(`kinds.${rule.kind}`);
          return (
            <div key={rule.id} className="flex items-center gap-3 px-4 py-3">
              <Switch
                checked={rule.enabled}
                disabled={!canWrite || save.isPending}
                onCheckedChange={(v) => toggle(rule, v)}
                aria-label={t("panel.toggleAria", { name })}
              />
              <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                <span className={rule.enabled ? "text-sm font-medium" : "text-sm font-medium text-muted-foreground"}>{name}</span>
                <Caption className="truncate">{summary(rule) || t(`kindHints.${rule.kind}`)}</Caption>
              </div>
              {rule.notifySlack && <Badge variant="outline">{t("panel.slack")}</Badge>}
              {!rule.enabled && <Badge variant="secondary">{t("panel.disabled")}</Badge>}
              {canWrite && (
                <>
                  <IconButton aria-label={t("panel.editAria", { name })} size="icon-sm" onClick={() => setEditing(rule)}>
                    <PencilIcon className="size-3.5" />
                  </IconButton>
                  <IconButton aria-label={t("panel.deleteAria", { name })} size="icon-sm" onClick={() => setDeleting(rule)}>
                    <Trash2Icon className="size-3.5" />
                  </IconButton>
                </>
              )}
            </div>
          );
        })}
      </CardContent>

      <Dialog open={editing !== null} onOpenChange={(open) => !open && setEditing(null)}>
        <DialogContent>
          {editing !== null && (
            <RuleForm
              key={editing === "new" ? "new" : editing.id}
              rule={editing === "new" ? undefined : editing}
              campaigns={campaigns}
              slackConnected={!!slackConnected}
              pending={save.isPending}
              onCancel={() => setEditing(null)}
              onSubmit={(input) =>
                save.mutate(
                  { id: editing === "new" ? undefined : editing.id, input },
                  {
                    onSuccess: () => {
                      toast.success(t("form.saved"));
                      setEditing(null);
                    },
                    onError: (err) => toast.error(t("form.saveError"), { description: err.message }),
                  },
                )
              }
            />
          )}
        </DialogContent>
      </Dialog>

      <Dialog open={!!deleting} onOpenChange={(open) => !open && setDeleting(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("delete.title")}</DialogTitle>
            <DialogDescription>{t("delete.description", { name: deleting ? t(`kinds.${deleting.kind}`) : "" })}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleting(null)}>
              {t("common:actions.cancel")}
            </Button>
            <Button variant="destructive" onClick={confirmDelete}>
              {t("common:actions.delete")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}

function RuleForm({
  rule,
  campaigns,
  slackConnected,
  pending,
  onCancel,
  onSubmit,
}: {
  rule?: AlertRule;
  campaigns: { id: string; name: string }[];
  slackConnected: boolean;
  pending: boolean;
  onCancel: () => void;
  onSubmit: (input: AlertRuleInput) => void;
}) {
  const { t } = useTranslation(["alerts", "common"]);
  const schema = React.useMemo(() => ruleSchema(t), [t]);
  const form = useForm<RuleValues>({ resolver: zodResolver(schema), defaultValues: toValues(rule) });
  const kind = useWatch({ control: form.control, name: "kind" });
  const errors = form.formState.errors;
  const thresholdLabel: Partial<Record<AlertKind, string>> = {
    spend_over: t("form.thresholdSpend"),
    roi_below: t("form.thresholdRoi"),
    ssl_expiring: t("form.thresholdDays"),
  };

  return (
    <form onSubmit={form.handleSubmit((v) => onSubmit(toInput(v)))} className="flex flex-col gap-4" noValidate>
      <DialogHeader>
        <DialogTitle>{rule ? t("form.editTitle") : t("form.createTitle")}</DialogTitle>
        <DialogDescription>{t(`kindHints.${kind}`)}</DialogDescription>
      </DialogHeader>

      <div className="grid gap-1.5">
        <Label htmlFor="alert-kind">{t("form.kind")}</Label>
        <Controller
          control={form.control}
          name="kind"
          render={({ field }) => (
            <Select
              value={field.value}
              onValueChange={(v) => {
                field.onChange(v);
                if (v === "ssl_expiring" && form.getValues("threshold") === "") form.setValue("threshold", "14");
              }}
            >
              <SelectTrigger id="alert-kind" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {ALERT_KINDS.map((k) => (
                  <SelectItem key={k} value={k}>
                    {t(`kinds.${k}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        />
      </div>

      {kindUsesCampaign(kind) && (
        <div className="grid gap-1.5">
          <Label htmlFor="alert-campaign">{t("form.campaign")}</Label>
          <Controller
            control={form.control}
            name="campaignId"
            render={({ field }) => (
              <Select value={field.value} onValueChange={field.onChange}>
                <SelectTrigger id="alert-campaign" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ALL_CAMPAIGNS}>{t("panel.allCampaigns")}</SelectItem>
                  {campaigns.map((c) => (
                    <SelectItem key={c.id} value={c.id}>
                      {c.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          />
        </div>
      )}

      {kindUsesThreshold(kind) && (
        <div className="grid gap-1.5">
          <Label htmlFor="alert-threshold">{thresholdLabel[kind]}</Label>
          <Input
            id="alert-threshold"
            inputMode="decimal"
            className="tabular-nums"
            aria-invalid={!!errors.threshold}
            {...form.register("threshold")}
          />
          {errors.threshold && <p className="text-xs text-danger">{errors.threshold.message}</p>}
        </div>
      )}

      {kind === "roi_below" && (
        <div className="grid gap-1.5">
          <Label htmlFor="alert-min-spend">{t("form.minSpend")}</Label>
          <Input
            id="alert-min-spend"
            inputMode="decimal"
            className="tabular-nums"
            aria-invalid={!!errors.minSpend}
            {...form.register("minSpend")}
          />
          {errors.minSpend && <p className="text-xs text-danger">{errors.minSpend.message}</p>}
        </div>
      )}

      <div className="flex flex-col gap-3">
        <Controller
          control={form.control}
          name="notifySlack"
          render={({ field }) => (
            <div className="flex flex-col gap-1">
              <div className="flex items-center gap-2">
                <Switch id="alert-slack" checked={field.value} onCheckedChange={field.onChange} />
                <Label htmlFor="alert-slack">{t("form.notifySlack")}</Label>
              </div>
              {field.value && !slackConnected && <p className="text-xs text-warning">{t("panel.slackNotConnected")}</p>}
            </div>
          )}
        />
        <Controller
          control={form.control}
          name="enabled"
          render={({ field }) => (
            <div className="flex items-center gap-2">
              <Switch id="alert-enabled" checked={field.value} onCheckedChange={field.onChange} />
              <Label htmlFor="alert-enabled">{t("form.enabled")}</Label>
            </div>
          )}
        />
      </div>

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>
          {t("common:actions.cancel")}
        </Button>
        <Button type="submit" disabled={pending}>
          {t("form.save")}
        </Button>
      </DialogFooter>
    </form>
  );
}
