"use client";

import * as React from "react";
import { toast } from "sonner";
import { PencilIcon, PlusIcon, StarIcon, XIcon } from "lucide-react";

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
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useReportPresetsStore } from "@/stores/report-presets";
import { dateRangeToPeriod, periodToDateRange, type ReportPreset } from "@/lib/mock/report-presets";
import type { ReportControlsState } from "@/features/analytics/report-controls";

/** Fixed "now" — matches the constant used throughout analytics-view.tsx so
 * applying a relative period is deterministic in this mock environment. */
const TODAY = new Date("2026-08-11T00:00:00Z");

export function ReportPresetBar({
  state,
  onApply,
}: {
  state: ReportControlsState;
  onApply: (patch: Partial<ReportControlsState>) => void;
}) {
  const presets = useReportPresetsStore((s) => s.presets);
  const createPreset = useReportPresetsStore((s) => s.createPreset);
  const updatePreset = useReportPresetsStore((s) => s.updatePreset);
  const deletePreset = useReportPresetsStore((s) => s.deletePreset);

  const [dialogTarget, setDialogTarget] = React.useState<ReportPreset | null | undefined>(undefined);
  const [name, setName] = React.useState("");

  function applyPreset(preset: ReportPreset) {
    onApply({
      dimensions: preset.dimensions,
      metrics: preset.metrics,
      groupBy: preset.groupBy,
      timezone: preset.timezone,
      dateRange: periodToDateRange(preset.period, TODAY),
    });
    toast("Preset applied", { description: preset.name });
  }

  function openSaveDialog(target: ReportPreset | null) {
    setName(target?.name ?? "");
    setDialogTarget(target);
  }

  function submitDialog() {
    const trimmed = name.trim();
    if (!trimmed) return;
    const input = {
      name: trimmed,
      dimensions: state.dimensions,
      metrics: state.metrics,
      groupBy: state.groupBy,
      timezone: state.timezone,
      period: dateRangeToPeriod(state.dateRange, TODAY),
    };
    if (dialogTarget) {
      updatePreset(dialogTarget.id, input);
      toast("Preset updated", { description: trimmed });
    } else {
      createPreset(input);
      toast("Preset saved", { description: trimmed });
    }
    setDialogTarget(undefined);
  }

  function handleDelete(preset: ReportPreset) {
    if (deletePreset(preset.id)) {
      toast("Preset deleted", { description: preset.name });
    }
  }

  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <span className="text-xs text-muted-foreground">Presets:</span>
      {presets.map((preset) => (
        <div key={preset.id} className="flex items-center overflow-hidden rounded-md border border-border">
          <button
            type="button"
            onClick={() => applyPreset(preset)}
            className="flex items-center gap-1 px-2 py-1 text-xs hover:bg-muted"
          >
            {preset.isDefault && <StarIcon className="size-3 fill-current text-warning" />}
            {preset.name}
          </button>
          {!preset.isDefault && (
            <>
              <IconButton
                aria-label={`Update ${preset.name} with the current view`}
                size="icon-sm"
                variant="ghost"
                className="rounded-none border-l border-border"
                onClick={() => openSaveDialog(preset)}
              >
                <PencilIcon className="size-3" />
              </IconButton>
              <IconButton
                aria-label={`Delete ${preset.name}`}
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
      ))}

      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="outline" size="sm" onClick={() => openSaveDialog(null)}>
            <PlusIcon className="size-3.5" /> Save as preset
          </Button>
        </TooltipTrigger>
        <TooltipContent>Saves the current columns, metrics, grouping, period, and timezone.</TooltipContent>
      </Tooltip>

      <Dialog open={dialogTarget !== undefined} onOpenChange={(open) => !open && setDialogTarget(undefined)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{dialogTarget ? "Update preset" : "Save current view as a preset"}</DialogTitle>
            <DialogDescription>
              {dialogTarget
                ? "Replaces this preset's saved configuration with the report you're looking at now."
                : "Reusable, team-scoped (§27.5) — reapplying it later reproduces this exact report."}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-1.5">
            <Label htmlFor="preset-name">Name</Label>
            <Input
              id="preset-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Source Performance — 7d"
              autoFocus
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogTarget(undefined)}>
              Cancel
            </Button>
            <Button onClick={submitDialog} disabled={!name.trim()}>
              {dialogTarget ? "Save changes" : "Save preset"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
