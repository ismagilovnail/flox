"use client";

import * as React from "react";
import { Controller, useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Mono } from "@/components/ui/typography";
import { formatMetric, type MetricFormat } from "@/features/analytics/registry";
import {
  METRIC_CATALOG,
  SHOW_IN_TARGETS,
  surfaceCanCompute,
  type ShowInTarget,
} from "@/lib/mock/custom-metrics-registry";
import type { CustomMetricStatus } from "@/lib/mock/custom-metrics";
import { evaluateFormula, validateFormula } from "@/lib/formula-engine";
import { MetricCatalogPanel } from "@/features/custom-metrics/metric-catalog-panel";
import { FormulaInput, type FormulaInputHandle } from "@/features/custom-metrics/formula-input";

const FORMATS: { value: MetricFormat; label: string }[] = [
  { value: "int", label: "Number" },
  { value: "currency", label: "Currency" },
  { value: "percent", label: "Percent" },
  { value: "ratio", label: "Ratio" },
];

/** Representative sample values covering every insertable registry metric, used
 * only for the form's live preview — not real data. */
const SAMPLE_VALUES: Record<string, number | null> = {
  clicks: 18500,
  uniqueClicks: 15200,
  conversions: 640,
  revenue: 8420.5,
  cost: 5100.25,
  profit: 3320.25,
  roi: 65.1,
  roas: 1.65,
  ctr: 42.3,
  cvr: 3.46,
  cpc: 0.28,
  cpa: 7.97,
  epc: 0.46,
  cpa_hold: 720,
  cpa_accept: 410,
  cpa_redep: 180,
  cpa_decline: 95,
  cpa_trash: 40,
  reg_to_ftd_rate: 56.9,
  ftd_to_redep_rate: 43.9,
  dep_to_redep: 30.5,
  total_deposits: 590,
  total_deposit_revenue: 6120.75,
  bots: 340,
  click_all: 18840,
  push_sent: 12000,
  push_delivered: 11400,
  push_opened: 2280,
  push_ctr: 20,
};

const customMetricFormSchema = z.object({
  name: z.string().min(2, "Name must be at least 2 characters").max(80),
  group: z.string().min(1, "Pick or type a group").max(40),
  format: z.enum(["int", "currency", "percent", "ratio"] as [MetricFormat, ...MetricFormat[]]),
  formula: z.string().min(1, "Enter a formula"),
  targets: z.array(z.enum(["report_builder", "campaigns_table", "offers_table", "sources_table"] as [ShowInTarget, ...ShowInTarget[]])),
  status: z.enum(["draft", "published"] as [CustomMetricStatus, ...CustomMetricStatus[]]),
});

export type CustomMetricFormValues = z.infer<typeof customMetricFormSchema>;

export function CustomMetricFormSheet({
  open,
  onOpenChange,
  defaultValues,
  existingGroups,
  title,
  submitLabel,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  defaultValues: Partial<CustomMetricFormValues>;
  existingGroups: string[];
  title: string;
  submitLabel: string;
  onSubmit: (values: CustomMetricFormValues) => void;
}) {
  const form = useForm<CustomMetricFormValues>({
    resolver: zodResolver(customMetricFormSchema),
    defaultValues: {
      name: "",
      group: existingGroups[0] ?? "",
      format: "currency",
      formula: "",
      targets: [],
      status: "draft",
      ...defaultValues,
    },
  });

  const { register, handleSubmit, control, setValue, formState } = form;
  const formulaInputRef = React.useRef<FormulaInputHandle>(null);

  const formula = useWatch({ control, name: "formula" });
  const targets = useWatch({ control, name: "targets" });
  const format = useWatch({ control, name: "format" });

  // Single source of truth, computed synchronously from `formula` in this same
  // render — see the note in formula-input.tsx on why this must not be lifted
  // from a child's effect instead.
  const validation = React.useMemo(() => validateFormula(formula, METRIC_CATALOG), [formula]);
  const preview = validation.valid ? evaluateFormula(formula, SAMPLE_VALUES) : null;

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full gap-0 overflow-y-auto sm:max-w-3xl" side="right">
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription>
            A formula built from registry metrics (§30.5) — division is always safe: dividing by zero shows as
            &ldquo;—&rdquo;, never an error.
          </SheetDescription>
        </SheetHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4 px-4 pb-4">
          <div className="grid gap-4 sm:grid-cols-3">
            <div className="grid gap-1.5">
              <Label htmlFor="cm-name">Name</Label>
              <Input id="cm-name" placeholder="Margin per Click" {...register("name")} aria-invalid={!!formState.errors.name} />
              {formState.errors.name && <p className="text-xs text-danger">{formState.errors.name.message}</p>}
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="cm-group">Group</Label>
              <Input id="cm-group" list="cm-group-options" placeholder="Profitability" {...register("group")} />
              <datalist id="cm-group-options">
                {existingGroups.map((g) => (
                  <option key={g} value={g} />
                ))}
              </datalist>
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="cm-format">Format</Label>
              <Controller
                control={control}
                name="format"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="cm-format" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {FORMATS.map((f) => (
                        <SelectItem key={f.value} value={f.value}>
                          {f.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            </div>
          </div>

          <Separator />

          <div className="grid gap-3 sm:grid-cols-[220px_1fr]">
            <div>
              <Label className="mb-1.5 block">Metrics</Label>
              <MetricCatalogPanel onInsert={(token) => formulaInputRef.current?.insertAtCursor(token)} />
            </div>
            <div className="flex flex-col gap-2">
              <Label>Formula</Label>
              <Controller
                control={control}
                name="formula"
                render={({ field }) => (
                  <FormulaInput
                    ref={formulaInputRef}
                    value={field.value}
                    onChange={field.onChange}
                    validation={validation}
                  />
                )}
              />
              <div className="rounded-md border border-border bg-muted/40 px-3 py-2 text-sm">
                <span className="text-xs text-muted-foreground">Preview (sample data): </span>
                <Mono>{validation.valid ? formatMetric(preview, format) : "—"}</Mono>
              </div>
            </div>
          </div>

          <Separator />

          <div className="flex flex-col gap-2">
            <Label>Show in</Label>
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
              {SHOW_IN_TARGETS.map((target) => {
                const canCompute = validation.valid && surfaceCanCompute(target.id, validation.usedMetricIds ?? []);
                const checked = targets.includes(target.id);
                const toggle = () => {
                  setValue(
                    "targets",
                    checked ? targets.filter((t) => t !== target.id) : [...targets, target.id],
                    { shouldDirty: true },
                  );
                };
                const control_ = (
                  <button
                    key={target.id}
                    type="button"
                    disabled={!canCompute}
                    onClick={toggle}
                    className="flex items-center gap-2 rounded-md border border-border px-2.5 py-2 text-left text-sm disabled:cursor-not-allowed disabled:opacity-50"
                  >
                    <span
                      className={
                        checked
                          ? "flex size-4 shrink-0 items-center justify-center rounded-sm border border-primary bg-primary"
                          : "flex size-4 shrink-0 items-center justify-center rounded-sm border border-input"
                      }
                    />
                    {target.label}
                  </button>
                );
                return canCompute ? (
                  control_
                ) : (
                  <Tooltip key={target.id}>
                    <TooltipTrigger asChild>{control_}</TooltipTrigger>
                    <TooltipContent>
                      {validation.valid
                        ? "This surface doesn't have all the metrics this formula needs."
                        : "Fix the formula first."}
                    </TooltipContent>
                  </Tooltip>
                );
              })}
            </div>
          </div>

          <div className="grid gap-1.5">
            <Label htmlFor="cm-status">Status</Label>
            <Controller
              control={control}
              name="status"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger id="cm-status" className="w-48">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="draft">Draft</SelectItem>
                    <SelectItem value="published">Published</SelectItem>
                  </SelectContent>
                </Select>
              )}
            />
            <p className="text-xs text-muted-foreground">
              Drafts are invisible in reports and pickers until published (§30.5).
            </p>
          </div>

          <SheetFooter className="mt-0 flex-row justify-end gap-2 p-0">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={!validation.valid || formState.isSubmitting}>
              {submitLabel}
            </Button>
          </SheetFooter>
        </form>
      </SheetContent>
    </Sheet>
  );
}
