"use client";

import { ChevronRightIcon, CopyIcon, InfoIcon } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { IconButton } from "@/components/ui/icon-button";
import { Tag } from "@/components/ui/tag";
import type { SimulateResult } from "@/lib/api/routing";
import { StreamSetTraceCard } from "@/features/routing-simulator/stream-set-trace";

const PIPELINE_STAGE_KEYS = [
  "request",
  "classification",
  "campaign",
  "streamSet",
  "filters",
  "flow",
  "destination",
] as const;

function PipelineStepper({ reachedIndex, t }: { reachedIndex: number; t: TFunction }) {
  return (
    <div className="flex flex-wrap items-center gap-1 text-xs">
      {PIPELINE_STAGE_KEYS.map((stage, i) => (
        <span key={stage} className="flex items-center gap-1">
          {i > 0 && <ChevronRightIcon className="size-3 text-muted-foreground" />}
          <span
            className={i <= reachedIndex ? "font-medium text-foreground" : "text-muted-foreground"}
          >
            {t(`result.pipeline.${stage}`)}
          </span>
        </span>
      ))}
    </div>
  );
}

export function SimulatorResult({ result, campaignName }: { result: SimulateResult; campaignName: string }) {
  const { t } = useTranslation("routingSimulator");
  const reachedIndex = result.flowCandidates.some((c) => c.selected)
    ? 6
    : result.matchedStreamSet
      ? 4
      : 3;

  function copyDestination() {
    if (!result.destination.url) return;
    navigator.clipboard.writeText(result.destination.url);
    toast(t("result.destinationCopiedToast"), { description: result.destination.url });
  }

  return (
    <div className="flex flex-col gap-4">
      <PipelineStepper reachedIndex={reachedIndex} t={t} />

      <Card size="sm">
        <CardHeader>
          <CardTitle className="text-sm">{t("result.campaignTitle")}</CardTitle>
          <CardDescription>{campaignName}</CardDescription>
        </CardHeader>
      </Card>

      <div className="flex flex-col gap-2">
        <h3 className="text-sm font-medium">{t("result.streamSetsTitle")}</h3>
        {result.streamSetEvaluations.map((e) => (
          <StreamSetTraceCard key={e.streamSetId} evaluation={e} />
        ))}
      </div>

      {result.matchedStreamSet && (
        <Card size="sm">
          <CardHeader>
            <CardTitle className="text-sm">{t("result.flowTitle")}</CardTitle>
            <CardDescription>{t("result.flowDescription")}</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-wrap gap-1.5">
            {result.flowCandidates.map((c) => (
              <Tag key={c.flowId} className={c.selected ? "border-primary bg-primary/10 text-primary" : undefined}>
                {c.selected && "✓ "}
                {t("result.flowTagLabel", { name: c.name, percent: `${c.normalizedPercent.toFixed(1)}%` })}
              </Tag>
            ))}
          </CardContent>
        </Card>
      )}

      <Card size="sm" className="ring-1 ring-primary/30">
        <CardHeader>
          <CardTitle className="text-sm">{t("result.destinationTitle")}</CardTitle>
          <CardDescription className="flex items-center gap-2">
            <Badge variant={result.destination.url ? "success" : "secondary"}>{result.destination.label}</Badge>
          </CardDescription>
        </CardHeader>
        <CardContent className="flex items-center gap-2">
          <span className="min-w-0 flex-1 truncate font-mono text-xs">
            {result.destination.url || t("result.noDestination")}
          </span>
          {result.destination.url && (
            <IconButton aria-label={t("result.copyDestinationAria")} size="icon-sm" onClick={copyDestination}>
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
