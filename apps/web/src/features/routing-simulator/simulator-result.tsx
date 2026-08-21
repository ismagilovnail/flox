"use client";

import { ChevronRightIcon, CopyIcon, InfoIcon } from "lucide-react";
import { toast } from "sonner";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { IconButton } from "@/components/ui/icon-button";
import { Tag } from "@/components/ui/tag";
import type { SimulateResult } from "@/lib/api/routing";
import { StreamSetTraceCard } from "@/features/routing-simulator/stream-set-trace";

const PIPELINE_STAGES = ["Request", "Classification", "Campaign", "Stream Set", "Filters", "Flow", "Destination"];

function PipelineStepper({ reachedIndex }: { reachedIndex: number }) {
  return (
    <div className="flex flex-wrap items-center gap-1 text-xs">
      {PIPELINE_STAGES.map((stage, i) => (
        <span key={stage} className="flex items-center gap-1">
          {i > 0 && <ChevronRightIcon className="size-3 text-muted-foreground" />}
          <span
            className={i <= reachedIndex ? "font-medium text-foreground" : "text-muted-foreground"}
          >
            {stage}
          </span>
        </span>
      ))}
    </div>
  );
}

export function SimulatorResult({ result, campaignName }: { result: SimulateResult; campaignName: string }) {
  const reachedIndex = result.flowCandidates.some((c) => c.selected)
    ? 6
    : result.matchedStreamSet
      ? 4
      : 3;

  function copyDestination() {
    if (!result.destination.url) return;
    navigator.clipboard.writeText(result.destination.url);
    toast("Destination URL copied", { description: result.destination.url });
  }

  return (
    <div className="flex flex-col gap-4">
      <PipelineStepper reachedIndex={reachedIndex} />

      <Card size="sm">
        <CardHeader>
          <CardTitle className="text-sm">Campaign</CardTitle>
          <CardDescription>{campaignName}</CardDescription>
        </CardHeader>
      </Card>

      <div className="flex flex-col gap-2">
        <h3 className="text-sm font-medium">Stream Sets — evaluated top-to-bottom, first match wins</h3>
        {result.streamSetEvaluations.map((e) => (
          <StreamSetTraceCard key={e.streamSetId} evaluation={e} />
        ))}
      </div>

      {result.matchedStreamSet && (
        <Card size="sm">
          <CardHeader>
            <CardTitle className="text-sm">Flow — weighted pick</CardTitle>
            <CardDescription>Normalized from each active flow&apos;s raw weight.</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-wrap gap-1.5">
            {result.flowCandidates.map((c) => (
              <Tag key={c.flowId} className={c.selected ? "border-primary bg-primary/10 text-primary" : undefined}>
                {c.selected && "✓ "}
                {c.name} · {c.normalizedPercent.toFixed(1)}%
              </Tag>
            ))}
          </CardContent>
        </Card>
      )}

      <Card size="sm" className="ring-1 ring-primary/30">
        <CardHeader>
          <CardTitle className="text-sm">Destination</CardTitle>
          <CardDescription className="flex items-center gap-2">
            <Badge variant={result.destination.url ? "success" : "secondary"}>{result.destination.label}</Badge>
          </CardDescription>
        </CardHeader>
        <CardContent className="flex items-center gap-2">
          <span className="min-w-0 flex-1 truncate font-mono text-xs">
            {result.destination.url || "No destination could be resolved — check campaign fallback."}
          </span>
          {result.destination.url && (
            <IconButton aria-label="Copy destination URL" size="icon-sm" onClick={copyDestination}>
              <CopyIcon className="size-3.5" />
            </IconButton>
          )}
        </CardContent>
      </Card>

      <Alert>
        <InfoIcon className="text-info" />
        <AlertDescription>{result.stickyNote}</AlertDescription>
      </Alert>
    </div>
  );
}
