"use client";

import * as React from "react";
import { ChevronDownIcon, ChevronRightIcon, CopyIcon, XIcon } from "lucide-react";

import { Card } from "@/components/ui/card";
import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";
import { Mono } from "@/components/ui/typography";
import { Switch } from "@/components/ui/switch";
import type { Flow } from "@/lib/mock/stream-sets";
import { FlowFunnel } from "@/features/stream-sets/flow-funnel";

export function FlowEditor({
  flow,
  normalizedPercent,
  fallbackUrl,
  onChange,
  onRemove,
  onDuplicate,
  canRemove,
}: {
  flow: Flow;
  normalizedPercent: number;
  fallbackUrl: string;
  onChange: (patch: Partial<Flow>) => void;
  onRemove: () => void;
  onDuplicate: () => void;
  canRemove: boolean;
}) {
  const [expanded, setExpanded] = React.useState(true);

  return (
    <Card size="sm" className="ring-1 ring-border">
      <div className="flex flex-wrap items-center gap-2 px-3">
        <IconButton
          aria-label={expanded ? "Collapse flow" : "Expand flow"}
          size="icon-sm"
          onClick={() => setExpanded((v) => !v)}
        >
          {expanded ? <ChevronDownIcon className="size-3.5" /> : <ChevronRightIcon className="size-3.5" />}
        </IconButton>
        <Input
          value={flow.name}
          onChange={(e) => onChange({ name: e.target.value })}
          placeholder="Flow name"
          className="h-7 w-40"
        />
        <div className="flex items-center gap-1.5">
          <Input
            type="number"
            min={0}
            value={flow.weight}
            onChange={(e) => onChange({ weight: e.target.valueAsNumber || 0 })}
            className="h-7 w-16 font-mono font-tabular"
          />
          <Mono className="text-xs text-muted-foreground">weight → {normalizedPercent.toFixed(1)}%</Mono>
        </div>
        <div className="ml-auto flex items-center gap-1.5">
          <Switch
            size="sm"
            checked={flow.active}
            onCheckedChange={(active) => onChange({ active })}
            aria-label={flow.active ? "Disable flow" : "Enable flow"}
          />
          <IconButton aria-label={`Duplicate ${flow.name}`} size="icon-sm" onClick={onDuplicate}>
            <CopyIcon className="size-3.5" />
          </IconButton>
          <IconButton aria-label={`Remove ${flow.name}`} size="icon-sm" onClick={onRemove} disabled={!canRemove}>
            <XIcon className="size-3.5" />
          </IconButton>
        </div>
      </div>

      {expanded && (
        <div className="px-3 pt-1">
          <FlowFunnel flow={flow} fallbackUrl={fallbackUrl} onChange={onChange} />
        </div>
      )}
    </Card>
  );
}
