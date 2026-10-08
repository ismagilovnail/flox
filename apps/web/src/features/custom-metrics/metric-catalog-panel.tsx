"use client";

import * as React from "react";
import { useTranslation } from "react-i18next";

import { Input } from "@/components/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { METRIC_CATALOG, METRIC_CATEGORIES } from "@/lib/custom-metrics-registry";
import { categoryLabel, metricLabel } from "@/features/custom-metrics/metric-label";

/** The searchable left panel: click a metric to insert its token at
 * the formula input's cursor. LTV entries are shown but disabled — clicking
 * does nothing, with a tooltip explaining why, rather than hiding them and
 * leaving the constraint undiscoverable. */
export function MetricCatalogPanel({ onInsert }: { onInsert: (token: string) => void }) {
  const { t } = useTranslation("customMetrics");
  const [query, setQuery] = React.useState("");

  const q = query.toLowerCase();
  const filtered = METRIC_CATALOG.filter(
    (m) => metricLabel(t, m).toLowerCase().includes(q) || m.id.toLowerCase().includes(q),
  );

  return (
    <div className="flex flex-col gap-2">
      <Input
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder={t("catalogSearch")}
        className="h-8"
      />
      <div className="flex max-h-80 flex-col gap-3 overflow-y-auto">
        {METRIC_CATEGORIES.map((category) => {
          const items = filtered.filter((m) => m.category === category);
          if (items.length === 0) return null;
          return (
            <div key={category} className="flex flex-col gap-0.5">
              <span className="px-1.5 text-xs font-medium text-muted-foreground">{categoryLabel(t, category)}</span>
              {items.map((metric) =>
                metric.insertable ? (
                  <button
                    key={metric.id}
                    type="button"
                    onClick={() => onInsert(`{${metric.id}}`)}
                    className="flex items-center justify-between gap-2 rounded-md px-1.5 py-1 text-left text-sm hover:bg-muted"
                  >
                    <span className="truncate">{metricLabel(t, metric)}</span>
                    {!metric.live && <span className="shrink-0 text-xs text-muted-foreground">{t("draftOnly")}</span>}
                  </button>
                ) : (
                  <Tooltip key={metric.id}>
                    <TooltipTrigger asChild>
                      <span
                        className={cn(
                          "flex cursor-not-allowed items-center justify-between gap-2 rounded-md px-1.5 py-1 text-sm text-muted-foreground/60",
                        )}
                      >
                        <span className="truncate">{metricLabel(t, metric)}</span>
                      </span>
                    </TooltipTrigger>
                    <TooltipContent>{t("ltvNotAllowed")}</TooltipContent>
                  </Tooltip>
                ),
              )}
            </div>
          );
        })}
        {filtered.length === 0 && <p className="px-1.5 py-2 text-xs text-muted-foreground">{t("catalogEmpty")}</p>}
      </div>
    </div>
  );
}
