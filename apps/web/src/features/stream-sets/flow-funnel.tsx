"use client";

import { FileStackIcon, LayoutTemplateIcon, SmartphoneIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { MacroPicker } from "@/components/shared/macro-picker";
import { EXTERNAL_PWA_MACROS, IOS_APP_MACROS } from "@/lib/macros";
import type { Landing } from "@/lib/api/landings";
import type { Pwa } from "@/lib/api/pwa";
import type { Postlanding } from "@/lib/api/postlanding";
import type { Network } from "@/lib/api/networks";
import type { Offer } from "@/lib/api/offers";
import { PWA_TYPES } from "@/lib/api/stream-sets";
import type { StreamSetFormValues } from "@/features/stream-sets/stream-set-schema";
import { FlowDestinationEditor } from "@/features/stream-sets/flow-destination-editor";
import { FlowNode } from "@/features/stream-sets/flow-node";
import type { FlowNodeSummaries } from "@/features/stream-sets/flow-summary";

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
  summaries,
  onChange,
}: {
  flow: FlowFormValue;
  fallbackUrl: string;
  networks: Network[];
  offers: Offer[];
  landings: Landing[];
  pwas: Pwa[];
  postlandings: Postlanding[];
  /** §25 per-node analytics — absent for a flow not saved yet. */
  summaries?: FlowNodeSummaries;
  onChange: (patch: Partial<FlowFormValue>) => void;
}) {
  const { t } = useTranslation("streamSets");
  const landingOption = landings.find((l) => l.id === flow.landing.landingId);
  const pwaOption = pwas.find((p) => p.id === flow.pwa.pwaId);
  const pwaType = flow.pwa.pwaType || PWA_TYPES[0];
  const postlandingOption = postlandings.find((p) => p.id === flow.postlanding.postlandingId);

  return (
    <div className="flex flex-col">
      <FlowNode
        icon={LayoutTemplateIcon}
        label={t("flowFunnel.landing")}
        enabled={flow.landing.enabled}
        onToggleEnabled={(enabled) => onChange({ landing: { ...flow.landing, enabled } })}
        configured={!!flow.landing.landingId}
        // Only an external landing's URL is a page; an internal one's is
        // its bundle's asset base, which isn't openable on its own.
        previewUrl={landingOption?.type === "external" ? landingOption.url : undefined}
        analytics={summaries?.landing}
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
        configured={pwaType === "internal" ? !!flow.pwa.pwaId : !!flow.pwa.url}
        previewUrl={pwaType === "internal" && pwaOption ? pwaOption.startUrl : undefined}
        analytics={summaries?.pwa}
      >
        <div className="flex w-full flex-col gap-2">
          <div className="flex flex-wrap items-center gap-2">
            <Select
              value={pwaType}
              onValueChange={(value) => onChange({ pwa: { ...flow.pwa, pwaType: value as FlowFormValue["pwa"]["pwaType"] } })}
            >
              <SelectTrigger size="sm" className="w-40" aria-label={t("flowFunnel.pwa")}>
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
            {pwaType !== "external" && (
              <Select
                value={flow.pwa.pwaId || NONE}
                onValueChange={(pwaId) => onChange({ pwa: { ...flow.pwa, pwaId: pwaId === NONE ? "" : pwaId } })}
              >
                <SelectTrigger size="sm" className="w-48" aria-label={pwaType === "ios_app" ? t("flowFunnel.pwaForOthers") : t("flowFunnel.choosePwaPlaceholder")}>
                  <SelectValue placeholder={pwaType === "ios_app" ? t("flowFunnel.pwaForOthers") : t("flowFunnel.choosePwaPlaceholder")} />
                </SelectTrigger>
                <SelectContent>
                  {pwaType === "ios_app" && <SelectItem value={NONE}>{t("flowFunnel.noPwaForOthers")}</SelectItem>}
                  {pwas.map((p) => (
                    <SelectItem key={p.id} value={p.id}>
                      {p.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          </div>
          {pwaType !== "internal" && (
            <div className="flex items-center gap-1">
              <Input
                value={flow.pwa.url}
                onChange={(e) => onChange({ pwa: { ...flow.pwa, url: e.target.value } })}
                placeholder={t(`flowFunnel.pwaUrlPlaceholder.${pwaType}`)}
                aria-label={t("flowFunnel.pwaUrlLabel")}
                className="h-8 font-mono text-xs"
                spellCheck={false}
              />
              <MacroPicker
                macros={pwaType === "external" ? EXTERNAL_PWA_MACROS : IOS_APP_MACROS}
                onInsert={(token) => onChange({ pwa: { ...flow.pwa, url: flow.pwa.url + token } })}
              />
            </div>
          )}
          <p className="text-xs text-muted-foreground">{t(`flowFunnel.pwaHint.${pwaType}`)}</p>
        </div>
      </FlowNode>

      <Connector />

      <FlowNode
        icon={FileStackIcon}
        label={t("flowFunnel.postlanding")}
        enabled={flow.postlanding.enabled}
        onToggleEnabled={(enabled) => onChange({ postlanding: { ...flow.postlanding, enabled } })}
        configured={!!flow.postlanding.postlandingId}
        previewUrl={postlandingOption?.url}
        analytics={summaries?.postlanding}
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

      <FlowDestinationEditor flow={flow} fallbackUrl={fallbackUrl} networks={networks} offers={offers} analytics={summaries?.destination} onChange={onChange} />
    </div>
  );
}
