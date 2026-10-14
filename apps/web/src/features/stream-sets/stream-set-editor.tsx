"use client";

import * as React from "react";
import { Controller, useFieldArray, useForm, useWatch, type Resolver } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { FilterIcon, PlusIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { MultiSelect } from "@/components/ui/multi-select";
import { genId } from "@/lib/id";
import type { FilterGroupNode } from "@/lib/filters";
import type { Network } from "@/lib/api/networks";
import type { Offer } from "@/lib/api/offers";
import type { Landing } from "@/lib/api/landings";
import type { Pwa } from "@/lib/api/pwa";
import type { Postlanding } from "@/lib/api/postlanding";
import type { Pixel } from "@/lib/api/pixels";
import { FilterPickerDialog } from "@/features/stream-sets/filter-picker-dialog";
import { FilterTreeSummary } from "@/features/stream-sets/filter-tree-summary";
import { FlowEditor } from "@/features/stream-sets/flow-editor";
import { buildStreamSetFormSchema, type StreamSetFormValues } from "@/features/stream-sets/stream-set-schema";
import { FLOW_SUMMARY_DAYS, flowNodeSummaries } from "@/features/stream-sets/flow-summary";
import { useFlowSummary } from "@/hooks/use-analytics-report";
import { useOrganization } from "@/hooks/use-organization";
import { useLocale } from "@/hooks/use-locale";
import { todayInZone } from "@/lib/timezones";

export type { StreamSetFormValues };

/** The stream set's own editing form, inline — expands directly under its
 * row (features/stream-sets/stream-set-row.tsx) instead of sliding out as
 * a separate Sheet. Same fields/validation/mutation shape the Sheet
 * version had; only the container changed, at the user's request to
 * match a reference tracker's layout (filters/pixels side by side,
 * rotation below, no slide-out panel). Collapsing the row (clicking its
 * name again) discards unsaved edits — there's no separate draft state,
 * same as the Sheet version's Cancel button did. */
export function StreamSetEditor({
  defaultValues,
  networks,
  offers,
  landings,
  pwas,
  postlandings,
  pixels,
  submitLabel,
  onSubmit,
  disabled = false,
}: {
  defaultValues: StreamSetFormValues;
  networks: Network[];
  offers: Offer[];
  landings: Landing[];
  pwas: Pwa[];
  postlandings: Postlanding[];
  pixels: Pixel[];
  submitLabel: string;
  onSubmit: (values: StreamSetFormValues) => void;
  /** Archived campaigns don't route traffic — a native <fieldset disabled>
   * locks every nested control (inputs, selects, the filter/flow buttons)
   * in one shot instead of threading a disabled prop through FlowEditor's
   * whole subtree. */
  disabled?: boolean;
}) {
  const { t } = useTranslation("streamSets");
  const [filterPickerOpen, setFilterPickerOpen] = React.useState(false);
  const form = useForm<StreamSetFormValues>({
    // rootFilter is a self-referential union (FilterCondition | FilterGroupNode); RHF's
    // Path<T> can't fully resolve that recursion, which makes zodResolver's inferred
    // type mismatch the plain StreamSetFormValues generic here. Cast — the mismatch is
    // a compile-time inference limitation only, not a runtime behavior difference.
    resolver: zodResolver(buildStreamSetFormSchema(t)) as Resolver<StreamSetFormValues>,
    defaultValues,
  });

  const {
    register,
    handleSubmit,
    control,
    setValue,
    formState: { errors, isSubmitting },
  } = form;

  const flowArray = useFieldArray({ control, name: "flows" });

  const flows = useWatch({ control, name: "flows" });
  const fallbackUrl = useWatch({ control, name: "fallbackUrl" });
  const rootFilter = useWatch({ control, name: "rootFilter" }) as FilterGroupNode;
  const weightSum = flows.reduce((sum, f) => sum + (f.weight || 0), 0);

  // §25 node analytics: the saved flows (defaultValues) over the last
  // FLOW_SUMMARY_DAYS of the organization's calendar. A flow added or
  // duplicated in this form has no id on the server yet — no summary.
  const locale = useLocale();
  const org = useOrganization();
  const timezone = org.isPending ? undefined : org.data?.timezone || "UTC";
  const savedFlowIds = React.useMemo(() => defaultValues.flows.map((f) => f.id), [defaultValues.flows]);
  const summaryTo = React.useMemo(() => todayInZone(timezone ?? "UTC"), [timezone]);
  const summaryFrom = React.useMemo(() => {
    const d = new Date(summaryTo);
    d.setUTCDate(d.getUTCDate() - (FLOW_SUMMARY_DAYS - 1));
    return d;
  }, [summaryTo]);
  const summary = useFlowSummary(savedFlowIds, summaryFrom, summaryTo, timezone, timezone !== undefined);

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-6 border-t border-border px-4 py-4">
      <fieldset disabled={disabled} className="contents">
      {disabled && (
        <p className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">{t("form.archivedNotice")}</p>
      )}
      <div className="grid gap-1.5 sm:max-w-md">
        <Label htmlFor="ss-name">{t("form.nameLabel")}</Label>
        <Input id="ss-name" {...register("name")} aria-invalid={!!errors.name} />
        {errors.name && <p className="text-xs text-danger">{errors.name.message}</p>}
      </div>

      <Separator />

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <div className="flex flex-col gap-3">
          <div className="flex items-start justify-between gap-3">
            <div>
              <h3 className="text-sm font-medium">{t("form.filtersTitle")}</h3>
              <p className="text-xs text-muted-foreground">{t("form.filtersDescription")}</p>
            </div>
            <Button type="button" variant="outline" size="sm" onClick={() => setFilterPickerOpen(true)}>
              <FilterIcon className="size-3.5" /> {t("form.addFilterButton")}
            </Button>
          </div>
          <FilterTreeSummary root={rootFilter} />
          {errors.rootFilter && <p className="text-xs text-danger">{t("form.filtersError")}</p>}
          <FilterPickerDialog
            open={filterPickerOpen}
            onOpenChange={setFilterPickerOpen}
            root={rootFilter}
            onApply={(next) => setValue("rootFilter", next, { shouldDirty: true, shouldValidate: true })}
          />
        </div>

        <div className="flex flex-col gap-3">
          <div>
            <h3 className="text-sm font-medium">{t("form.pixelsTitle")}</h3>
            <p className="text-xs text-muted-foreground">{t("form.pixelsDescription")}</p>
          </div>
          <Controller
            control={control}
            name="pixelIds"
            render={({ field }) => (
              <MultiSelect
                label={t("form.pixelsTitle")}
                options={pixels.map((p) => ({ value: p.id, label: p.name }))}
                selected={field.value}
                onChange={field.onChange}
                className="w-full"
              />
            )}
          />
        </div>
      </div>

      <Separator />

      <div className="flex flex-col gap-3">
        <div>
          <h3 className="text-sm font-medium">{t("form.flowsTitle")}</h3>
          <p className="text-xs text-muted-foreground">{t("form.flowsDescription")}</p>
          <p className="text-xs text-muted-foreground" aria-live="polite">
            {summary.isError
              ? t("flowSummary.error")
              : summary.isPending
                ? t("flowSummary.loading")
                : t("flowSummary.period", { days: FLOW_SUMMARY_DAYS })}
          </p>
        </div>

        {/* Per-flow edits go through setValue(`flows.${index}`, ...), not
            flowArray.update(index, ...): RHF's own docs say update()
            unregisters and re-registers the row, which remounts its
            subtree on every keystroke/selection. Reproduced live —
            the offer picker inside FlowDestinationEditor would select
            correctly, then get silently reset to empty a moment
            later, because the Select itself was being torn down and
            rebuilt mid-interaction. setValue patches the field in
            place with no remount. */}
        <div className="flex flex-col gap-2">
          {flowArray.fields.map((field, index) => {
            const flow = flows[index];
            if (!flow) return null;
            return (
              <FlowEditor
                key={field.id}
                flow={flow}
                normalizedPercent={weightSum > 0 ? (flow.weight / weightSum) * 100 : 0}
                fallbackUrl={fallbackUrl}
                networks={networks}
                offers={offers}
                landings={landings}
                pwas={pwas}
                postlandings={postlandings}
                summaries={summary.data?.[flow.id] ? flowNodeSummaries(summary.data[flow.id], t, locale) : undefined}
                onChange={(patch) => setValue(`flows.${index}`, { ...flow, ...patch }, { shouldDirty: true, shouldValidate: true })}
                onRemove={() => flowArray.remove(index)}
                onDuplicate={() =>
                  flowArray.insert(index + 1, {
                    ...flow,
                    id: genId(),
                    name: t("form.flowCopySuffix", { name: flow.name }),
                  })
                }
                canRemove={flowArray.fields.length > 1}
              />
            );
          })}
        </div>
        {errors.flows?.message && <p className="text-xs text-danger">{errors.flows.message}</p>}

        <div className="flex items-center justify-between">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() =>
              flowArray.append({
                id: genId(),
                name: `Flow ${flowArray.fields.length + 1}`,
                active: true,
                weight: 0,
                landing: { enabled: false, landingId: "", asPwa: false },
                pwa: { enabled: false, pwaId: "", pwaType: "" },
                postlanding: { enabled: false, postlandingId: "" },
                destination: { kind: "offer", networkId: networks[0]?.id ?? "", offerId: "" },
              })
            }
          >
            <PlusIcon className="size-3.5" /> {t("form.addFlow")}
          </Button>
          <span className="text-xs font-mono text-muted-foreground">
            {t("form.totalWeight", { weight: weightSum })}
          </span>
        </div>
      </div>

      <Separator />

      <div className="grid gap-1.5 sm:max-w-md">
        <Label htmlFor="ss-fallback">{t("form.fallbackLabel")}</Label>
        <Input
          id="ss-fallback"
          placeholder={t("form.fallbackPlaceholder")}
          {...register("fallbackUrl")}
          aria-invalid={!!errors.fallbackUrl}
        />
        {errors.fallbackUrl && <p className="text-xs text-danger">{errors.fallbackUrl.message}</p>}
        <p className="text-xs text-muted-foreground">{t("form.fallbackHint")}</p>
      </div>

      <div className="flex justify-end">
        <Button type="submit" disabled={disabled || isSubmitting}>
          {submitLabel}
        </Button>
      </div>
      </fieldset>
    </form>
  );
}
