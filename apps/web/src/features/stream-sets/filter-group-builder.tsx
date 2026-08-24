"use client";

import { PlusIcon, XIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { IconButton } from "@/components/ui/icon-button";
import { cn } from "@/lib/utils";
import {
  addConditionToGroup,
  addGroupToGroup,
  removeNode,
  updateCondition,
  updateGroupJoiner,
  type FilterGroupNode,
} from "@/lib/filters";
import { FilterConditionEditor } from "@/features/stream-sets/filter-condition-editor";

/** Recursive AND/OR rule tree per §22-23 — `root` is the whole tree, `group`
 * is the node this instance renders; both are threaded through recursion so
 * every level can produce an updated whole-tree value via `onRootChange`. */
export function FilterGroupBuilder({
  root,
  group,
  onRootChange,
  depth = 0,
}: {
  root: FilterGroupNode;
  group: FilterGroupNode;
  onRootChange: (next: FilterGroupNode) => void;
  depth?: number;
}) {
  const { t } = useTranslation("streamSets");
  return (
    <div className={cn("flex flex-col gap-2", depth > 0 && "border-l-2 border-border pl-4")}>
      <div className="flex items-center justify-between">
        <button
          type="button"
          onClick={() => onRootChange(updateGroupJoiner(root, group.id, group.joiner === "AND" ? "OR" : "AND"))}
          className={cn(
            "rounded-md border px-2 py-0.5 text-[0.6875rem] font-semibold uppercase tracking-wide transition-colors",
            group.joiner === "AND"
              ? "border-info/30 bg-info/10 text-info hover:bg-info/20"
              : "border-warning/30 bg-warning/10 text-warning hover:bg-warning/20",
          )}
        >
          {group.joiner === "AND" ? t("filterGroupBuilder.matchAll") : t("filterGroupBuilder.matchAny")}
        </button>
        {depth > 0 && (
          <IconButton
            aria-label={t("filterGroupBuilder.removeGroupAria")}
            size="icon-sm"
            onClick={() => onRootChange(removeNode(root, group.id))}
          >
            <XIcon className="size-3.5" />
          </IconButton>
        )}
      </div>

      <div className="flex flex-col gap-2">
        {group.children.length === 0 && (
          <p className="text-xs text-muted-foreground">{t("filterGroupBuilder.emptyGroupText")}</p>
        )}
        {group.children.map((child) =>
          child.type === "condition" ? (
            <FilterConditionEditor
              key={child.id}
              condition={child}
              onChange={(patch) => onRootChange(updateCondition(root, child.id, patch))}
              onRemove={() => onRootChange(removeNode(root, child.id))}
            />
          ) : (
            <FilterGroupBuilder key={child.id} root={root} group={child} onRootChange={onRootChange} depth={depth + 1} />
          ),
        )}
      </div>

      <div className="flex gap-2">
        <Button type="button" variant="outline" size="sm" onClick={() => onRootChange(addConditionToGroup(root, group.id))}>
          <PlusIcon className="size-3.5" /> {t("filterGroupBuilder.addCondition")}
        </Button>
        <Button type="button" variant="outline" size="sm" onClick={() => onRootChange(addGroupToGroup(root, group.id))}>
          <PlusIcon className="size-3.5" /> {t("filterGroupBuilder.addGroup")}
        </Button>
      </div>
    </div>
  );
}
