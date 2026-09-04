"use client";

import { useTranslation } from "react-i18next";

import { Badge } from "@/components/ui/badge";
import { FilterChip } from "@/components/ui/filter-chip";
import { FilterGroup } from "@/components/ui/filter-group";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { FILTER_OPERATOR_I18N_KEY, countConditions, describeFilterTree, type FilterGroupNode, type FilterOperator } from "@/lib/filters";

/** Read-only chip rendering of a filter tree's top level, with the plain-
 * language description as a tooltip. Shared by the stream-set row's
 * collapsed summary and the editor's compact filters block so the two
 * never drift apart. */
export function FilterTreeSummary({ root }: { root: FilterGroupNode }) {
  const { t } = useTranslation("streamSets");

  if (root.children.length === 0) {
    return <p className="text-xs text-muted-foreground">{t("row.noFilters")}</p>;
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div className="w-fit">
          <FilterGroup joiner={root.joiner}>
            {root.children.map((child) =>
              child.type === "condition" ? (
                <FilterChip
                  key={child.id}
                  field={child.field}
                  operator={t(FILTER_OPERATOR_I18N_KEY[child.operator as FilterOperator])}
                  value={child.operator === "BETWEEN" ? `${child.value}–${child.valueTo}` : child.value || "—"}
                />
              ) : (
                <Badge key={child.id} variant="outline">
                  {child.joiner === "AND" ? t("filterGroupBuilder.matchAll") : t("filterGroupBuilder.matchAny")} (
                  {countConditions(child)})
                </Badge>
              ),
            )}
          </FilterGroup>
        </div>
      </TooltipTrigger>
      <TooltipContent>{describeFilterTree(root)}</TooltipContent>
    </Tooltip>
  );
}
