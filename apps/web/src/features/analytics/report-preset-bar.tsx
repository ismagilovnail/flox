"use client";

import * as React from "react";
import { toast } from "sonner";
import { PencilIcon, PlusIcon, StarIcon, XIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

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
  const { t } = useTranslation("analytics");
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
    toast(t("presets.toast.applied"), { description: preset.name });
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
      toast(t("presets.toast.updated"), { description: trimmed });
    } else {
      createPreset(input);
      toast(t("presets.toast.saved"), { description: trimmed });
    }
    setDialogTarget(undefined);
  }

  function handleDelete(preset: ReportPreset) {
    if (deletePreset(preset.id)) {
      toast(t("presets.toast.deleted"), { description: preset.name });
    }
  }

  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <span className="text-xs text-muted-foreground">{t("presets.label")}</span>
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
      ))}

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
