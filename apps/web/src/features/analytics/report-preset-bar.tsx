"use client";

import * as React from "react";
import { toast } from "sonner";
import { PencilIcon, PlusIcon, RefreshCwIcon, SaveIcon, StarIcon, XIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  EMPTY_REPORT_PRESETS,
  useCreateReportPreset,
  useDeleteReportPreset,
  useReportPresets,
  useUpdateReportPreset,
} from "@/hooks/use-report-presets";
import { dateRangeToPeriod, periodToDateRange, type ReportPreset } from "@/lib/report-presets";
import type { FilterCondition } from "@/features/analytics/aggregate";
import type { ReportControlsState } from "@/features/analytics/report-controls";

/** The chosen preset survives a reload; per browser, like any view preference
 * (presets themselves are org-wide and live on the server). */
const ACTIVE_PRESET_KEY = "flox.analytics.activePreset";

function readActivePreset(): string | null {
  try {
    return window.localStorage.getItem(ACTIVE_PRESET_KEY);
  } catch {
    return null;
  }
}

function writeActivePreset(id: string | null) {
  try {
    if (id) window.localStorage.setItem(ACTIVE_PRESET_KEY, id);
    else window.localStorage.removeItem(ACTIVE_PRESET_KEY);
  } catch {
    // storage blocked (private window): the selection just won't persist
  }
}

const isoDay = (d: Date) => d.toISOString().slice(0, 10);

function sameRange(a: ReportControlsState["dateRange"], b: { from: Date; to: Date }): boolean {
  if (!a?.from) return false;
  return isoDay(a.from) === isoDay(b.from) && isoDay(a.to ?? a.from) === isoDay(b.to);
}

function sameWidths(a: Record<string, number>, b: Record<string, number>): boolean {
  const ka = Object.keys(a);
  return ka.length === Object.keys(b).length && ka.every((k) => a[k] === b[k]);
}

function sameFilters(a: FilterCondition[], b: FilterCondition[]): boolean {
  const key = (f: FilterCondition) => `${f.dimension}=${f.value}`;
  const bs = new Set(b.map(key));
  return a.length === b.length && a.every((f) => bs.has(key(f)));
}

/** Whether the report still shows exactly what `preset` defines. Once the
 * operator edits anything (dimensions, metrics, filters, grouping, period,
 * timezone) the preset is shown as modified and can be saved over. */
function matchesPreset(state: ReportControlsState, preset: ReportPreset): boolean {
  const same = (a: string[], b: string[]) => a.length === b.length && a.every((v, i) => v === b[i]);
  return (
    same(state.dimensions, preset.dimensions) &&
    same(state.metrics, preset.metrics) &&
    state.groupBy === preset.groupBy &&
    state.timezone === preset.timezone &&
    sameFilters(state.filters, preset.filters ?? []) &&
    same(state.columnOrder ?? [], preset.columnOrder ?? []) &&
    sameWidths(state.columnWidths ?? {}, preset.columnWidths ?? {}) &&
    sameRange(state.dateRange, periodToDateRange(preset.period, new Date()))
  );
}

export function ReportPresetBar({
  state,
  onApply,
}: {
  state: ReportControlsState;
  onApply: (patch: Partial<ReportControlsState>) => void;
}) {
  const { t } = useTranslation("analytics");
  const presetsQuery = useReportPresets();
  const presets = presetsQuery.data?.presets ?? EMPTY_REPORT_PRESETS;
  const createPreset = useCreateReportPreset();
  const updatePreset = useUpdateReportPreset();
  const deletePreset = useDeleteReportPreset();

  const [dialogTarget, setDialogTarget] = React.useState<ReportPreset | null | undefined>(undefined);
  const [name, setName] = React.useState("");

  // Read lazily: the preset chips only render once the list has loaded, so the
  // server-rendered skeleton and this first client render still agree.
  const [activeId, setActiveId] = React.useState<string | null>(() =>
    typeof window === "undefined" ? null : readActivePreset(),
  );
  const restored = React.useRef(false);

  function toPatch(preset: ReportPreset): Partial<ReportControlsState> {
    return {
      dimensions: preset.dimensions,
      metrics: preset.metrics,
      filters: preset.filters ?? [],
      columnOrder: preset.columnOrder ?? [],
      columnWidths: preset.columnWidths ?? {},
      groupBy: preset.groupBy,
      timezone: preset.timezone,
      dateRange: periodToDateRange(preset.period, new Date()),
    };
  }

  function applyPreset(preset: ReportPreset) {
    onApply(toPatch(preset));
    setActiveId(preset.id);
    writeActivePreset(preset.id);
    toast(t("presets.toast.applied"), { description: preset.name });
  }

  // Once the list has loaded, re-apply the preset chosen last time (silently).
  // A page opened from a "View statistics" link (?dim=…) keeps that link's report.
  React.useEffect(() => {
    if (restored.current || !presetsQuery.isSuccess) return;
    restored.current = true;
    if (new URLSearchParams(window.location.search).has("dim")) return;
    const preset = activeId ? presetsQuery.data.presets.find((p) => p.id === activeId) : undefined;
    if (preset) onApply(toPatch(preset));
    else if (activeId) writeActivePreset(null); // deleted since
  });

  function openSaveDialog(target: ReportPreset | null) {
    setName(target?.name ?? "");
    setDialogTarget(target);
  }

  function buildInput(presetName: string) {
    return {
      name: presetName,
      dimensions: state.dimensions,
      metrics: state.metrics,
      filters: state.filters,
      columnOrder: state.columnOrder ?? [],
      columnWidths: state.columnWidths ?? {},
      groupBy: state.groupBy,
      timezone: state.timezone,
      period: dateRangeToPeriod(state.dateRange, new Date()),
    };
  }

  /** Writes the report on screen over the active preset — no dialog, the name stays. */
  function saveChangesToActive(preset: ReportPreset) {
    updatePreset.mutate(
      { id: preset.id, input: buildInput(preset.name) },
      {
        onSuccess: () => toast(t("presets.toast.updated"), { description: preset.name }),
        onError: (err) => toast.error(t("presets.toast.updated"), { description: err.message }),
      },
    );
  }

  function submitDialog() {
    const trimmed = name.trim();
    if (!trimmed) return;
    const input = buildInput(trimmed);
    if (dialogTarget) {
      updatePreset.mutate(
        { id: dialogTarget.id, input },
        {
          onSuccess: () => {
            setActiveId(dialogTarget.id);
            writeActivePreset(dialogTarget.id);
            toast(t("presets.toast.updated"), { description: trimmed });
          },
          onError: (err) => toast.error(t("presets.toast.updated"), { description: err.message }),
        },
      );
    } else {
      createPreset.mutate(input, {
        onSuccess: (created) => {
          // The report on screen is exactly what was just saved: it is the active preset.
          setActiveId(created.id);
          writeActivePreset(created.id);
          toast(t("presets.toast.saved"), { description: trimmed });
        },
        onError: (err) => toast.error(t("presets.toast.saved"), { description: err.message }),
      });
    }
    setDialogTarget(undefined);
  }

  function handleDelete(preset: ReportPreset) {
    deletePreset.mutate(preset.id, {
      onSuccess: () => {
        if (activeId === preset.id) {
          setActiveId(null);
          writeActivePreset(null);
        }
        toast(t("presets.toast.deleted"), { description: preset.name });
      },
      onError: (err) => toast.error(t("presets.toast.deleted"), { description: err.message }),
    });
  }

  const activePreset = presets.find((p) => p.id === activeId);

  if (presetsQuery.isPending) {
    return (
      <div className="flex items-center gap-1.5">
        <span className="text-xs text-muted-foreground">{t("presets.label")}</span>
        <Skeleton className="h-6 w-32 rounded-md" />
        <Skeleton className="h-6 w-24 rounded-md" />
      </div>
    );
  }

  if (presetsQuery.isError) {
    return (
      <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
        <span>{t("presets.label")}</span>
        <span>{presetsQuery.error.message}</span>
        <IconButton
          aria-label={t("presets.retryAria")}
          size="icon-sm"
          variant="ghost"
          onClick={() => presetsQuery.refetch()}
        >
          <RefreshCwIcon className="size-3" />
        </IconButton>
      </div>
    );
  }

  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <span className="text-xs text-muted-foreground">{t("presets.label")}</span>
      {presets.map((preset) => {
        const selected = activeId === preset.id;
        const active = selected && matchesPreset(state, preset);
        const modified = selected && !active;
        return (
        <div
          key={preset.id}
          className={cn(
            "flex items-center overflow-hidden rounded-md border",
            active && "border-primary bg-primary/10",
            modified && "border-dashed border-primary",
            !selected && "border-border",
          )}
        >
          <button
            type="button"
            aria-pressed={active}
            onClick={() => applyPreset(preset)}
            className={cn("flex items-center gap-1 px-2 py-1 text-xs hover:bg-muted", active && "font-medium")}
          >
            {preset.isDefault && <StarIcon className="size-3 fill-current text-warning" />}
            {preset.name}
            {modified && <span className="text-primary" title={t("presets.modified")}>•</span>}
          </button>
          {!preset.isDefault && (
            <>
              <IconButton
                aria-label={t("presets.updateAria", { name: preset.name })}
                size="icon-sm"
                variant="ghost"
                className="rounded-none border-l border-border"
                onClick={() => openSaveDialog(preset)}
              >
                <PencilIcon className="size-3" />
              </IconButton>
              <IconButton
                aria-label={t("presets.deleteAria", { name: preset.name })}
                size="icon-sm"
                variant="ghost"
                className="rounded-none border-l border-border"
                onClick={() => handleDelete(preset)}
              >
                <XIcon className="size-3" />
              </IconButton>
            </>
          )}
        </div>
        );
      })}

      {activePreset && !activePreset.isDefault && !matchesPreset(state, activePreset) && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button size="sm" disabled={updatePreset.isPending} onClick={() => saveChangesToActive(activePreset)}>
              <SaveIcon className="size-3.5" /> {t("presets.saveChanges")}
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t("presets.saveChangesTooltip", { name: activePreset.name })}</TooltipContent>
        </Tooltip>
      )}

      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="outline" size="sm" onClick={() => openSaveDialog(null)}>
            <PlusIcon className="size-3.5" /> {t("presets.save")}
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t("presets.saveTooltip")}</TooltipContent>
      </Tooltip>

      <Dialog open={dialogTarget !== undefined} onOpenChange={(open) => !open && setDialogTarget(undefined)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{dialogTarget ? t("presets.dialog.updateTitle") : t("presets.dialog.createTitle")}</DialogTitle>
            <DialogDescription>
              {dialogTarget ? t("presets.dialog.updateDescription") : t("presets.dialog.createDescription")}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-1.5">
            <Label htmlFor="preset-name">{t("presets.dialog.nameLabel")}</Label>
            <Input
              id="preset-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t("presets.dialog.namePlaceholder")}
              autoFocus
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogTarget(undefined)}>
              {t("presets.dialog.cancel")}
            </Button>
            <Button onClick={submitDialog} disabled={!name.trim()}>
              {dialogTarget ? t("presets.dialog.saveChanges") : t("presets.dialog.savePreset")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
