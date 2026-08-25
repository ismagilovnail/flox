"use client";

import { FileStackIcon, LayoutTemplateIcon, SmartphoneIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import type { Landing } from "@/lib/api/landings";
import type { Pwa } from "@/lib/api/pwa";
import type { Postlanding } from "@/lib/api/postlanding";
import type { Network } from "@/lib/api/networks";
import type { Offer } from "@/lib/api/offers";
import { PWA_TYPES } from "@/lib/api/stream-sets";
import type { StreamSetFormValues } from "@/features/stream-sets/stream-set-schema";
import { FlowDestinationEditor } from "@/features/stream-sets/flow-destination-editor";
import { FlowNode } from "@/features/stream-sets/flow-node";

type FlowFormValue = StreamSetFormValues["flows"][number];

// Radix Select's controlled `value` behaves unreliably once it's ever been
// an empty string (see flow-destination-editor.tsx's own NO_OFFER
// sentinel for the reproduced bug and reasoning) — every optional picker
// below uses the same "no selection" sentinel rather than "".
const NONE = "__none__";

function Connector() {
  return <div className="ml-[15px] h-3 w-px bg-border" />;
}

/** Renders the full §25 funnel: Landing -> PWA -> Postlanding -> the
 * Offer-or-Redirect Destination (delegated to FlowDestinationEditor) ->
 * Fallback. Restores what FlowDestinationEditor's own doc comment
 * describes dropping, now that internal/landing, internal/pwa, and
 * internal/postlanding are real — see docs/stream-sets.md. Per-flow
 * Pixels stays out: pixels attach to the Stream Set, not the Flow. */
export function FlowFunnel({
  flow,
  fallbackUrl,
  networks,
  offers,
  landings,
  pwas,
  postlandings,
  onChange,
}: {
  flow: FlowFormValue;
  fallbackUrl: string;
  networks: Network[];
  offers: Offer[];
  landings: Landing[];
  pwas: Pwa[];
  postlandings: Postlanding[];
  onChange: (patch: Partial<FlowFormValue>) => void;
}) {
  const { t } = useTranslation("streamSets");
  const landingOption = landings.find((l) => l.id === flow.landing.landingId);
  const pwaOption = pwas.find((p) => p.id === flow.pwa.pwaId);
  const postlandingOption = postlandings.find((p) => p.id === flow.postlanding.postlandingId);

  return (
    <div className="flex flex-col">
      <FlowNode
        icon={LayoutTemplateIcon}
        label={t("flowFunnel.landing")}
        enabled={flow.landing.enabled}
        onToggleEnabled={(enabled) => onChange({ landing: { ...flow.landing, enabled } })}
        configured={!!flow.landing.landingId}
        previewUrl={landingOption?.url}
      >
        <Select
          value={flow.landing.landingId || NONE}
          onValueChange={(landingId) => onChange({ landing: { ...flow.landing, landingId: landingId === NONE ? "" : landingId } })}
        >
          <SelectTrigger size="sm" className="w-44">
            <SelectValue placeholder={t("flowFunnel.chooseLandingPlaceholder")} />
          </SelectTrigger>
          <SelectContent>
            {landings.map((l) => (
              <SelectItem key={l.id} value={l.id}>
                {l.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <input
            type="checkbox"
            checked={flow.landing.asPwa}
            onChange={(e) => onChange({ landing: { ...flow.landing, asPwa: e.target.checked } })}
            className="size-3.5 accent-primary"
          />
          {t("flowFunnel.showAsPwa")}
        </label>
      </FlowNode>

      <Connector />

      <FlowNode
        icon={SmartphoneIcon}
        label={t("flowFunnel.pwa")}
        enabled={flow.pwa.enabled}
        onToggleEnabled={(enabled) => onChange({ pwa: { ...flow.pwa, enabled, pwaType: flow.pwa.pwaType || PWA_TYPES[0] } })}
        configured={!!flow.pwa.pwaId}
        previewUrl={pwaOption ? pwaOption.startUrl : undefined}
      >
        <Select
          value={flow.pwa.pwaId || NONE}
          onValueChange={(pwaId) => onChange({ pwa: { ...flow.pwa, pwaId: pwaId === NONE ? "" : pwaId } })}
        >
          <SelectTrigger size="sm" className="w-40">
            <SelectValue placeholder={t("flowFunnel.choosePwaPlaceholder")} />
          </SelectTrigger>
          <SelectContent>
            {pwas.map((p) => (
              <SelectItem key={p.id} value={p.id}>
                {p.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          value={flow.pwa.pwaType || PWA_TYPES[0]}
          onValueChange={(pwaType) => onChange({ pwa: { ...flow.pwa, pwaType: pwaType as FlowFormValue["pwa"]["pwaType"] } })}
        >
          <SelectTrigger size="sm" className="w-32">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {PWA_TYPES.map((type) => (
              <SelectItem key={type} value={type}>
                {t(`flowFunnel.pwaType.${type}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </FlowNode>

      <Connector />

      <FlowNode
        icon={FileStackIcon}
        label={t("flowFunnel.postlanding")}
        enabled={flow.postlanding.enabled}
        onToggleEnabled={(enabled) => onChange({ postlanding: { ...flow.postlanding, enabled } })}
        configured={!!flow.postlanding.postlandingId}
        previewUrl={postlandingOption?.url}
      >
        <Select
          value={flow.postlanding.postlandingId || NONE}
          onValueChange={(postlandingId) =>
            onChange({ postlanding: { ...flow.postlanding, postlandingId: postlandingId === NONE ? "" : postlandingId } })
          }
        >
          <SelectTrigger size="sm" className="w-44">
            <SelectValue placeholder={t("flowFunnel.choosePostlandingPlaceholder")} />
          </SelectTrigger>
          <SelectContent>
            {postlandings.map((p) => (
              <SelectItem key={p.id} value={p.id}>
                {p.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </FlowNode>

      <Connector />

      <FlowDestinationEditor flow={flow} fallbackUrl={fallbackUrl} networks={networks} offers={offers} onChange={onChange} />
    </div>
  );
}
