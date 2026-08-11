"use client";

import * as React from "react";
import type { DateRange } from "react-day-picker";
import { ArrowDownIcon, ArrowUpIcon, PlusIcon } from "lucide-react";

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
import { DIMENSIONS, METRICS, TIMEZONES, type DimensionKey, type MetricKey } from "@/features/analytics/registry";
import type { FilterCondition } from "@/features/analytics/aggregate";
import { dimensionValues } from "@/lib/mock/analytics";

export type ReportControlsState = {
  dateRange?: DateRange;
  timezone: string;
  dimensions: DimensionKey[];
  metrics: MetricKey[];
  filters: FilterCondition[];
  groupBy: DimensionKey;
  sort: { key: string; dir: "asc" | "desc" };
  compare: boolean;
};

const dimensionLabel = (key: DimensionKey) => DIMENSIONS.find((d) => d.key === key)?.label ?? key;

function AddFilterPopover({
  onAdd,
}: {
  onAdd: (f: FilterCondition) => void;
}) {
  const [open, setOpen] = React.useState(false);
  const [dimension, setDimension] = React.useState<DimensionKey>("country");
  const [value, setValue] = React.useState<string>(dimensionValues("country")[0]);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="gap-1.5">
          <PlusIcon className="size-3.5" />
          Filter
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-64" align="start">
        <div className="flex flex-col gap-3">
          <div className="grid gap-1.5">
            <Label>Dimension</Label>
            <Select
              value={dimension}
              onValueChange={(v) => {
                const dim = v as DimensionKey;
                setDimension(dim);
                setValue(dimensionValues(dim)[0]);
              }}
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {DIMENSIONS.map((d) => (
                  <SelectItem key={d.key} value={d.key}>
                    {d.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-1.5">
            <Label>Value</Label>
            <Select value={value} onValueChange={setValue}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {dimensionValues(dimension).map((v) => (
                  <SelectItem key={v} value={v}>
                    {v}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <Button
            size="sm"
            onClick={() => {
              onAdd({ dimension, value });
              setOpen(false);
            }}
          >
            Add filter
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
  const sortOptions = [
    ...state.dimensions.map((d) => ({ value: d, label: dimensionLabel(d) })),
    ...state.metrics.map((m) => ({
      value: m,
      label: METRICS.find((x) => x.key === m)?.label ?? m,
    })),
  ];

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <DateRangePicker
          value={state.dateRange}
          onChange={(dateRange) => onChange({ dateRange })}
        />

        <Select value={state.timezone} onValueChange={(timezone) => onChange({ timezone })}>
          <SelectTrigger className="w-44" size="sm">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {TIMEZONES.map((tz) => (
              <SelectItem key={tz} value={tz}>
                {tz}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <MultiSelect
          label="Dimensions"
          options={DIMENSIONS.map((d) => ({ value: d.key, label: d.label }))}
          selected={state.dimensions}
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
          label="Metrics"
          options={METRICS.map((m) => ({ value: m.key, label: m.label }))}
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
            <span className="text-muted-foreground">Group by:</span>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {state.dimensions.map((d) => (
              <SelectItem key={d} value={d}>
                {dimensionLabel(d)}
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
              <span className="text-muted-foreground">Sort:</span>
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
            aria-label={state.sort.dir === "asc" ? "Sort descending" : "Sort ascending"}
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
            Compare
          </Label>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <AddFilterPopover onAdd={(f) => onChange({ filters: [...state.filters, f] })} />
        {state.filters.length > 0 && (
          <FilterGroup joiner="AND">
            {state.filters.map((f, i) => (
              <FilterChip
                key={`${f.dimension}-${f.value}-${i}`}
                field={dimensionLabel(f.dimension)}
                operator="is"
                value={f.value}
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
