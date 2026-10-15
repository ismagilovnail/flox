"use client";

import * as React from "react";
import { SearchIcon, XIcon } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { DateRange } from "react-day-picker";

import { DateRangePicker } from "@/components/ui/date-range-picker";
import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";

/**
 * The filters of a server-paged log (DataGrid's toolbar): the date range
 * and an exact click id, applied on Enter (or blur) so typing does not
 * query on every key.
 */
export function LogGridToolbar({
  range,
  onRangeChange,
  clickId,
  onClickIdChange,
  timezone,
}: {
  range: { from: Date; to: Date };
  onRangeChange: (range: DateRange | undefined) => void;
  clickId: string;
  onClickIdChange: (clickId: string) => void;
  timezone: string;
}) {
  const { t } = useTranslation("common");
  const [draft, setDraft] = React.useState(clickId);
  const [applied, setApplied] = React.useState(clickId);
  // The applied value can change from outside (a reset): follow it.
  if (applied !== clickId) {
    setApplied(clickId);
    setDraft(clickId);
  }

  const apply = () => {
    if (draft.trim() !== clickId) onClickIdChange(draft);
  };

  return (
    <>
      <DateRangePicker value={range} onChange={onRangeChange} timezone={timezone} />
      <form
        className="relative w-72"
        role="search"
        onSubmit={(e) => {
          e.preventDefault();
          apply();
        }}
      >
        <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
        <Input
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={apply}
          placeholder={t("dataGrid.clickIdPlaceholder")}
          aria-label={t("dataGrid.clickIdPlaceholder")}
          className="h-8 pr-8 pl-8 font-mono text-xs"
          spellCheck={false}
        />
        {draft !== "" && (
          <IconButton
            type="button"
            aria-label={t("dataGrid.clearClickId")}
            size="icon-sm"
            variant="ghost"
            className="absolute right-0.5 top-1/2 -translate-y-1/2"
            onClick={() => {
              setDraft("");
              onClickIdChange("");
            }}
          >
            <XIcon className="size-3.5" />
          </IconButton>
        )}
      </form>
    </>
  );
}
