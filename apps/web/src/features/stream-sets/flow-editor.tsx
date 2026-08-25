"use client";

import * as React from "react";
import { ChevronDownIcon, ChevronRightIcon, CopyIcon, XIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Card } from "@/components/ui/card";
import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";
import { Mono } from "@/components/ui/typography";
import { Switch } from "@/components/ui/switch";
import type { Network } from "@/lib/api/networks";
import type { Offer } from "@/lib/api/offers";
import type { Landing } from "@/lib/api/landings";
import type { Pwa } from "@/lib/api/pwa";
import type { Postlanding } from "@/lib/api/postlanding";
import type { StreamSetFormValues } from "@/features/stream-sets/stream-set-schema";
import { FlowFunnel } from "@/features/stream-sets/flow-funnel";
import { TagBadgeList } from "@/features/tags/tag-badge-list";

type FlowFormValue = StreamSetFormValues["flows"][number];

export function FlowEditor({
  flow,
  normalizedPercent,
  fallbackUrl,
  networks,
  offers,
  landings,
  pwas,
  postlandings,
  onChange,
  onRemove,
  onDuplicate,
  canRemove,
}: {
  flow: FlowFormValue;
  normalizedPercent: number;
  fallbackUrl: string;
  networks: Network[];
  offers: Offer[];
  landings: Landing[];
  pwas: Pwa[];
  postlandings: Postlanding[];
  onChange: (patch: Partial<FlowFormValue>) => void;
  onRemove: () => void;
  onDuplicate: () => void;
  canRemove: boolean;
}) {
  const { t } = useTranslation("streamSets");
  const [expanded, setExpanded] = React.useState(true);

  return (
    <Card size="sm" className="ring-1 ring-border">
      <div className="flex flex-wrap items-center gap-2 px-3">
        <IconButton
          aria-label={expanded ? t("flowEditor.collapseAria") : t("flowEditor.expandAria")}
          size="icon-sm"
          onClick={() => setExpanded((v) => !v)}
        >
          {expanded ? <ChevronDownIcon className="size-3.5" /> : <ChevronRightIcon className="size-3.5" />}
        </IconButton>
        <Input
          value={flow.name}
          onChange={(e) => onChange({ name: e.target.value })}
          placeholder={t("flowEditor.namePlaceholder")}
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
          <Mono className="text-xs text-muted-foreground">
            {t("flowEditor.weightLabel", { percent: `${normalizedPercent.toFixed(1)}%` })}
          </Mono>
        </div>
        <TagBadgeList entityType="flow" entityId={flow.id} />
        <div className="ml-auto flex items-center gap-1.5">
          <Switch
            size="sm"
            checked={flow.active}
            onCheckedChange={(active) => onChange({ active })}
            aria-label={flow.active ? t("flowEditor.disableAria") : t("flowEditor.enableAria")}
          />
          <IconButton aria-label={t("flowEditor.duplicateAria", { name: flow.name })} size="icon-sm" onClick={onDuplicate}>
            <CopyIcon className="size-3.5" />
          </IconButton>
          <IconButton
            aria-label={t("flowEditor.removeAria", { name: flow.name })}
            size="icon-sm"
            onClick={onRemove}
            disabled={!canRemove}
          >
            <XIcon className="size-3.5" />
          </IconButton>
        </div>
      </div>

      {expanded && (
        <div className="px-3 pt-1">
          <FlowFunnel
            flow={flow}
            fallbackUrl={fallbackUrl}
            networks={networks}
            offers={offers}
            landings={landings}
            pwas={pwas}
            postlandings={postlandings}
            onChange={onChange}
          />
        </div>
      )}
    </Card>
  );
}
