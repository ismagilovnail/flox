"use client";

import * as React from "react";
import { CircleHelpIcon, FilterIcon, PlusIcon, SearchIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import {
  FIELD_GROUPS,
  FIELD_GROUP_I18N_KEY,
  addFieldConditionToGroup,
  countConditions,
  countFilterValues,
  type FilterGroupNode,
} from "@/lib/filters";
import { FilterGroupBuilder } from "@/features/stream-sets/filter-group-builder";
import { FilterReferenceSheet } from "@/features/stream-sets/filter-reference-sheet";

/** The "как в документации" field catalog: search + categorized fields on
 * the left (click to append a condition), the same recursive
 * FilterGroupBuilder used inline elsewhere on the right (so add-group/
 * remove/AND-OR-toggle logic isn't duplicated), Отменить/Применить commit
 * the draft back to the caller only on Apply. */
export function FilterPickerDialog({
  open,
  onOpenChange,
  root,
  onApply,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  root: FilterGroupNode;
  onApply: (next: FilterGroupNode) => void;
}) {
  const { t } = useTranslation("streamSets");
  const [draft, setDraft] = React.useState(root);
  const [search, setSearch] = React.useState("");
  const [helpOpen, setHelpOpen] = React.useState(false);

  // Re-seed the draft from the committed tree on the closed->open edge only
  // (React's "adjust state during render" pattern, not an effect — an
  // effect-based reset would clobber in-progress edits on every `root`
  // change while the dialog is open, since the parent form re-renders on
  // each keystroke).
  const [wasOpen, setWasOpen] = React.useState(open);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) {
      setDraft(root);
      setSearch("");
    }
  }

  const query = search.trim().toLowerCase();
  const filteredGroups = FIELD_GROUPS.map((g) => ({
    ...g,
    fields: query ? g.fields.filter((f) => f.toLowerCase().includes(query)) : g.fields,
  })).filter((g) => g.fields.length > 0);

  const conditionCount = countConditions(draft);
  const valueCount = countFilterValues(draft);

  function handleApply() {
    onApply(draft);
    onOpenChange(false);
  }

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="flex max-h-[85vh] w-[calc(100%-2rem)] max-w-5xl flex-col gap-0 overflow-hidden p-0 sm:max-w-5xl">
          <DialogDescription className="sr-only">{t("filterPicker.description")}</DialogDescription>
          <div className="flex items-center justify-between gap-4 border-b border-border py-3 pr-12 pl-4">
            <div className="flex shrink-0 items-center gap-3">
              <DialogTitle className="flex items-center gap-1.5 text-base">
                <FilterIcon className="size-4" /> {t("filterPicker.title")}
              </DialogTitle>
              <Button type="button" variant="ghost" size="sm" onClick={() => setHelpOpen(true)}>
                <CircleHelpIcon className="size-3.5" /> {t("filterReference.trigger")}
              </Button>
            </div>
            <div className="relative w-full max-w-64">
              <SearchIcon className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder={t("filterPicker.searchPlaceholder")}
                className="h-8 pl-8"
              />
            </div>
          </div>

          <div className="grid flex-1 grid-cols-[220px_1fr] overflow-hidden">
            <div className="flex flex-col gap-3 overflow-y-auto border-r border-border p-3">
              {filteredGroups.length === 0 ? (
                <p className="px-1 text-xs text-muted-foreground">{t("filterPicker.noFieldsFound")}</p>
              ) : (
                filteredGroups.map((g) => (
                  <div key={g.label} className="flex flex-col gap-0.5">
                    <p className="px-2 text-[0.6875rem] font-semibold tracking-wide text-muted-foreground uppercase">
                      {t(FIELD_GROUP_I18N_KEY[g.label])}
                    </p>
                    {g.fields.map((f) => (
                      <button
                        key={f}
                        type="button"
                        onClick={() => setDraft((d) => addFieldConditionToGroup(d, d.id, f))}
                        className="flex items-center justify-between rounded-md px-2 py-1.5 text-left font-mono text-xs text-foreground transition-colors hover:bg-muted"
                      >
                        {f}
                        <PlusIcon className="size-3.5 text-muted-foreground" />
                      </button>
                    ))}
                  </div>
                ))
              )}
            </div>

            <div className="flex flex-col gap-3 overflow-y-auto p-4">
              <div className="flex items-center gap-2">
                <h4 className="text-sm font-medium">{t("filterPicker.activeFiltersTitle")}</h4>
                <span className="rounded-full bg-muted px-1.5 py-0.5 text-[0.6875rem] font-medium text-muted-foreground">
                  {conditionCount}
                </span>
              </div>

              {conditionCount === 0 ? (
                <div className="flex flex-1 flex-col items-center justify-center gap-1.5 py-10 text-center">
                  <div className="flex size-10 items-center justify-center rounded-full bg-muted">
                    <FilterIcon className="size-4 text-muted-foreground" />
                  </div>
                  <p className="text-sm font-medium text-foreground">{t("filterPicker.emptyTitle")}</p>
                  <p className="max-w-xs text-xs text-muted-foreground">{t("filterPicker.emptyDescription")}</p>
                </div>
              ) : (
                <FilterGroupBuilder root={draft} group={draft} onRootChange={setDraft} />
              )}
            </div>
          </div>

          <div className="flex items-center justify-between gap-3 border-t border-border bg-muted/30 px-4 py-3">
            <span className="text-xs text-muted-foreground">
              {t("filterPicker.summary", { count: conditionCount, values: valueCount })}
            </span>
            <div className="flex gap-2">
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                {t("actions.cancel", { ns: "common" })}
              </Button>
              <Button type="button" onClick={handleApply}>
                {t("filterPicker.apply")}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <FilterReferenceSheet open={helpOpen} onOpenChange={setHelpOpen} />
    </>
  );
}
