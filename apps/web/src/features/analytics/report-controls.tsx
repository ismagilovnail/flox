"use client";

import * as React from "react";
import type { DateRange } from "react-day-picker";
import { ArrowDownIcon, ArrowUpIcon, PlusIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { DateRangePicker } from "@/components/ui/date-range-picker";
import { MultiSelect } from "@/components/ui/multi-select";
import { Button } from "@/components/ui/button";
import { IconButton } from "@/components/ui/icon-button";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
import { FilterGroup } from "@/components/ui/filter-group";
import { FilterChip } from "@/components/ui/filter-chip";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { FILTER_DIMENSIONS, MAX_REPORT_DIMENSIONS, REPORT_DIMENSIONS, METRICS, dimensionLabel as dimensionLabelFor, metricLabel as metricLabelFor, type DimensionKey, type MetricKey } from "@/features/analytics/registry";
import type { FilterCondition } from "@/features/analytics/aggregate";
import { TimezoneSelect } from "@/components/ui/timezone-select";
import { useDimensionValues } from "@/hooks/use-analytics-report";
import { useDimensionValueLabel } from "@/features/analytics/use-dimension-value-label";

export type ReportControlsState = {
  dateRange?: DateRange;
  timezone: string;
  dimensions: DimensionKey[];
  metrics: MetricKey[];
  filters: FilterCondition[];
  /** Dragged table column order (ids); [] is the natural order. Saved with presets. */
  columnOrder: string[];
  /** Dragged table column widths in px by column id; {} is natural widths. Saved with presets. */
  columnWidths: Record<string, number>;
  /** Ids of the columns hidden in the table; [] shows every column. Saved with presets. */
  hiddenColumns: string[];
  groupBy: DimensionKey;
  sort: { key: string; dir: "asc" | "desc" };
  compare: boolean;
};

function AddFilterPopover({
  onAdd,
  dateRange,
}: {
  onAdd: (f: FilterCondition) => void;
  dateRange: { from: Date; to: Date };
}) {
  const { t } = useTranslation("analytics");
  const [open, setOpen] = React.useState(false);
  const [dimension, setDimension] = React.useState<DimensionKey>("country");
  // The user's own explicit pick, if they've made one for the current
  // values list — undefined means "not picked yet, or the values list
  // moved out from under an earlier pick" (e.g. the real list finally
  // loaded after the mock placeholder), in which case `value` below
  // derives the first available option instead. Deriving during render
  // rather than syncing via an effect avoids the cascading-render
  // pattern React's own rules warn against for this exact "reset a
  // selection when its source list changes" shape.
  const [manualValue, setManualValue] = React.useState<string | undefined>(undefined);
  const valueLabel = useDimensionValueLabel();

  // Only real values: while they load (or if the request fails) the picker is
  // empty and the add button disabled, never offering a made-up value that would
  // silently filter the report down to nothing.
  const valuesQuery = useDimensionValues(dimension, dateRange.from, dateRange.to, { enabled: open });
  const values = valuesQuery.isSuccess ? valuesQuery.data : [];
  const value = manualValue && values.includes(manualValue) ? manualValue : (values[0] ?? "");

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="gap-1.5">
          <PlusIcon className="size-3.5" />
          {t("controls.filter")}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-64" align="start">
        <div className="flex flex-col gap-3">
          <div className="grid gap-1.5">
            <Label>{t("controls.dimension")}</Label>
            <Select
              value={dimension}
              onValueChange={(v) => {
                setDimension(v as DimensionKey);
                setManualValue(undefined);
              }}
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {FILTER_DIMENSIONS.map((d) => (
                  <SelectItem key={d.key} value={d.key}>
                    {dimensionLabelFor(t, d.key)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-1.5">
            <Label>{t("controls.value")}</Label>
            <Select value={value} onValueChange={setManualValue} disabled={values.length === 0}>
              <SelectTrigger className="w-full">
                <SelectValue placeholder={values.length === 0 ? t("controls.noValues") : undefined} />
              </SelectTrigger>
              <SelectContent>
                {values.map((v) => (
                  <SelectItem key={v} value={v}>
                    {valueLabel(dimension, v)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <Button
            size="sm"
            disabled={values.length === 0}
            onClick={() => {
              onAdd({ dimension, value });
              setOpen(false);
            }}
          >
            {t("controls.addFilter")}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}

export function ReportControls({
  state,
  onChange,
}: {
  state: ReportControlsState;
  onChange: (patch: Partial<ReportControlsState>) => void;
}) {
  const { t } = useTranslation(["analytics", "streamSets"]);
  const valueLabel = useDimensionValueLabel();
  const sortOptions = [
    ...state.dimensions.map((d) => ({ value: d, label: dimensionLabelFor(t, d) })),
    ...state.metrics.map((m) => ({
      value: m,
      label: metricLabelFor(t, m),
    })),
  ];

  // Computed once per mount (empty deps), matching analytics-view.tsx's
  // own React.useMemo(() => new Date(), []) pattern — a plain `new
  // Date()`/`Date.now()` call directly in the render body is an impure
  // render (React's own rules), since every re-render would produce a
  // different value. Only ever used as a fallback for the filter
  // popover's own date-range query when state.dateRange itself is
  // somehow unset (AnalyticsView always initializes it, so this is a
  // defensive default, not the common path).
  const fallbackTo = React.useMemo(() => new Date(), []);
  const fallbackFrom = React.useMemo(() => new Date(fallbackTo.getTime() - 29 * 24 * 60 * 60 * 1000), [fallbackTo]);

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <DateRangePicker
          value={state.dateRange}
          onChange={(dateRange) => onChange({ dateRange })}
        />

        <TimezoneSelect value={state.timezone} onValueChange={(timezone) => onChange({ timezone })} className="w-52" />

        <MultiSelect
          label={t("controls.dimensionsLabel")}
          options={REPORT_DIMENSIONS.map((d) => ({ value: d.key, label: dimensionLabelFor(t, d.key) }))}
          selected={state.dimensions}
          max={MAX_REPORT_DIMENSIONS}
          onChange={(values) => {
            const dimensions = values as DimensionKey[];
            onChange({
              dimensions,
              groupBy: dimensions.includes(state.groupBy) ? state.groupBy : dimensions[0],
            });
          }}
          className="w-48"
        />

        <MultiSelect
          label={t("controls.metricsLabel")}
          options={METRICS.map((m) => ({ value: m.key, label: metricLabelFor(t, m.key) }))}
          selected={state.metrics}
          onChange={(values) => onChange({ metrics: values as MetricKey[] })}
          className="w-48"
        />

        <Select
          value={state.groupBy}
          onValueChange={(groupBy) => onChange({ groupBy: groupBy as DimensionKey })}
          disabled={state.dimensions.length === 0}
        >
          <SelectTrigger className="w-44" size="sm">
            <span className="text-muted-foreground">{t("controls.groupBy")}</span>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {state.dimensions.map((d) => (
              <SelectItem key={d} value={d}>
                {dimensionLabelFor(t, d)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <div className="flex items-center gap-1">
          <Select
            value={state.sort.key}
            onValueChange={(key) => onChange({ sort: { ...state.sort, key } })}
          >
            <SelectTrigger className="w-40" size="sm">
              <span className="text-muted-foreground">{t("controls.sort")}</span>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {sortOptions.map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <IconButton
            aria-label={state.sort.dir === "asc" ? t("controls.sortDescendingAria") : t("controls.sortAscendingAria")}
            variant="outline"
            size="icon-sm"
            onClick={() =>
              onChange({ sort: { ...state.sort, dir: state.sort.dir === "asc" ? "desc" : "asc" } })
            }
          >
            {state.sort.dir === "asc" ? (
              <ArrowUpIcon className="size-3.5" />
            ) : (
              <ArrowDownIcon className="size-3.5" />
            )}
          </IconButton>
        </div>

        <div className="flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1.5">
          <Switch
            id="compare"
            checked={state.compare}
            onCheckedChange={(compare) => onChange({ compare })}
          />
          <Label htmlFor="compare" className="text-xs">
            {t("controls.compare")}
          </Label>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <AddFilterPopover
          onAdd={(f) => onChange({ filters: [...state.filters, f] })}
          dateRange={{
            from: state.dateRange?.from ?? fallbackFrom,
            to: state.dateRange?.to ?? state.dateRange?.from ?? fallbackTo,
          }}
        />
        {state.filters.length > 0 && (
          <FilterGroup joiner="AND">
            {state.filters.map((f, i) => (
              <FilterChip
                key={`${f.dimension}-${f.value}-${i}`}
                field={dimensionLabelFor(t, f.dimension)}
                operator={t("filters.operators.is", { ns: "streamSets" })}
                value={valueLabel(f.dimension, f.value)}
                onRemove={() =>
                  onChange({ filters: state.filters.filter((_, idx) => idx !== i) })
                }
              />
            ))}
          </FilterGroup>
        )}
      </div>
    </div>
  );
}
