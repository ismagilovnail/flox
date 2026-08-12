"use client";

import * as React from "react";

import { Input } from "@/components/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { METRIC_CATALOG, METRIC_CATEGORIES } from "@/lib/mock/custom-metrics-registry";

/** The searchable left panel (§30.5): click a metric to insert its token at
 * the formula input's cursor. LTV entries are shown but disabled — clicking
 * does nothing, with a tooltip explaining why, rather than hiding them and
 * leaving the constraint undiscoverable. */
export function MetricCatalogPanel({ onInsert }: { onInsert: (token: string) => void }) {
  const [query, setQuery] = React.useState("");

  const filtered = METRIC_CATALOG.filter(
    (m) => m.label.toLowerCase().includes(query.toLowerCase()) || m.id.toLowerCase().includes(query.toLowerCase()),
  );

  return (
    <div className="flex flex-col gap-2">
      <Input
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder="Search metrics..."
        className="h-8"
      />
      <div className="flex max-h-80 flex-col gap-3 overflow-y-auto">
        {METRIC_CATEGORIES.map((category) => {
          const items = filtered.filter((m) => m.category === category);
          if (items.length === 0) return null;
          return (
            <div key={category} className="flex flex-col gap-0.5">
              <span className="px-1.5 text-xs font-medium text-muted-foreground">{category}</span>
              {items.map((metric) =>
                metric.insertable ? (
                  <button
                    key={metric.id}
                    type="button"
                    onClick={() => onInsert(`{${metric.id}}`)}
                    className="flex items-center justify-between gap-2 rounded-md px-1.5 py-1 text-left text-sm hover:bg-muted"
                  >
                    <span className="truncate">{metric.label}</span>
                    {!metric.live && <span className="text-xs text-muted-foreground">draft-only</span>}
                  </button>
                ) : (
                  <Tooltip key={metric.id}>
                    <TooltipTrigger asChild>
                      <span
                        className={cn(
                          "flex cursor-not-allowed items-center justify-between gap-2 rounded-md px-1.5 py-1 text-sm text-muted-foreground/60",
                        )}
                      >
                        <span className="truncate">{metric.label}</span>
                      </span>
                    </TooltipTrigger>
                    <TooltipContent>LTV metrics can&apos;t be used in custom metric formulas (§30.5).</TooltipContent>
                  </Tooltip>
                ),
              )}
            </div>
          );
        })}
        {filtered.length === 0 && <p className="px-1.5 py-2 text-xs text-muted-foreground">No metrics match.</p>}
      </div>
    </div>
  );
}
