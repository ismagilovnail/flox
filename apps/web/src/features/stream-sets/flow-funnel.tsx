"use client";

import { ArrowUpRightIcon, FileStackIcon, LayoutTemplateIcon, LifeBuoyIcon, SmartphoneIcon, TargetIcon } from "lucide-react";

import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { formatInt } from "@/lib/format";
import { cn } from "@/lib/utils";
import { PWA_TYPES } from "@/lib/mock/flow-entities";
import type { Flow } from "@/lib/mock/stream-sets";
import { useNetworksStore } from "@/stores/networks";
import { useOffersStore } from "@/stores/offers";
import { useLandingsStore } from "@/stores/landings";
import { usePwasStore } from "@/stores/pwas";
import { usePostlandingsStore } from "@/stores/postlandings";
import { FlowNode } from "@/features/stream-sets/flow-node";

function Connector() {
  return <div className="ml-[15px] h-3 w-px bg-border" />;
}

/** Small deterministic mock stat per node — real per-node analytics arrive
 * once the tracker (Phase 16+) and Analytics Explorer (Phase 5) are wired
 * to the same event stream; this is a placeholder demoing the §25
 * "analytics summary" node capability. */
function nodeAnalytics(flowId: string, stage: string) {
  let seed = 0;
  const key = flowId + stage;
  for (let i = 0; i < key.length; i++) seed = (seed * 31 + key.charCodeAt(i)) | 0;
  seed = Math.abs(seed) % 10000;
  const views = 400 + (seed % 4000);
  const continued = 55 + (seed % 35);
  return `${formatInt(views)} views · ${continued}% continued`;
}

export function FlowFunnel({
  flow,
  fallbackUrl,
  onChange,
}: {
  flow: Flow;
  fallbackUrl: string;
  onChange: (patch: Partial<Flow>) => void;
}) {
  const networks = useNetworksStore((s) => s.networks);
  const offers = useOffersStore((s) => s.offers);
  const landings = useLandingsStore((s) => s.landings);
  const pwas = usePwasStore((s) => s.pwas);
  const postlandings = usePostlandingsStore((s) => s.postlandings);
  const landingOption = landings.find((l) => l.id === flow.landing.landingId);
  const pwaOption = pwas.find((p) => p.id === flow.pwa.pwaId);
  const postlandingOption = postlandings.find((p) => p.id === flow.postlanding.postlandingId);
  const destination = flow.destination;
  const networkOffers = destination.kind === "offer" ? offers.filter((o) => o.networkId === destination.networkId) : [];

  return (
    <div className="flex flex-col">
      <FlowNode
        icon={LayoutTemplateIcon}
        label="Landing"
        enabled={flow.landing.enabled}
        onToggleEnabled={(enabled) => onChange({ landing: { ...flow.landing, enabled } })}
        configured={!!flow.landing.landingId}
        previewUrl={landingOption?.url}
        analytics={flow.landing.enabled ? nodeAnalytics(flow.id, "landing") : undefined}
      >
        <Select
          value={flow.landing.landingId || undefined}
          onValueChange={(landingId) => onChange({ landing: { ...flow.landing, landingId } })}
        >
          <SelectTrigger size="sm" className="w-44">
            <SelectValue placeholder="Choose landing" />
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
          Show as PWA
        </label>
      </FlowNode>

      <Connector />

      <FlowNode
        icon={SmartphoneIcon}
        label="PWA"
        enabled={flow.pwa.enabled}
        onToggleEnabled={(enabled) => onChange({ pwa: { ...flow.pwa, enabled } })}
        configured={!!flow.pwa.pwaId}
        previewUrl={pwaOption ? `https://pwa.floxlink.io${pwaOption.startUrl}` : undefined}
        analytics={flow.pwa.enabled ? nodeAnalytics(flow.id, "pwa") : undefined}
      >
        <Select value={flow.pwa.pwaId || undefined} onValueChange={(pwaId) => onChange({ pwa: { ...flow.pwa, pwaId } })}>
          <SelectTrigger size="sm" className="w-40">
            <SelectValue placeholder="Choose PWA" />
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
          value={flow.pwa.pwaType}
          onValueChange={(pwaType) => onChange({ pwa: { ...flow.pwa, pwaType: pwaType as Flow["pwa"]["pwaType"] } })}
        >
          <SelectTrigger size="sm" className="w-28">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {PWA_TYPES.map((t) => (
              <SelectItem key={t} value={t}>
                {t.replace("_", " ")}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </FlowNode>

      <Connector />

      <FlowNode
        icon={FileStackIcon}
        label="Postlanding"
        enabled={flow.postlanding.enabled}
        onToggleEnabled={(enabled) => onChange({ postlanding: { ...flow.postlanding, enabled } })}
        configured={!!flow.postlanding.postlandingId}
        previewUrl={postlandingOption?.url}
        analytics={flow.postlanding.enabled ? nodeAnalytics(flow.id, "postlanding") : undefined}
      >
        <Select
          value={flow.postlanding.postlandingId || undefined}
          onValueChange={(postlandingId) => onChange({ postlanding: { ...flow.postlanding, postlandingId } })}
        >
          <SelectTrigger size="sm" className="w-44">
            <SelectValue placeholder="Choose postlanding" />
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

      <FlowNode
        icon={flow.destination.kind === "offer" ? TargetIcon : ArrowUpRightIcon}
        label={flow.destination.kind === "offer" ? "Offer" : "Redirect"}
        toggleable={false}
        enabled
        configured={flow.destination.kind === "offer" ? !!flow.destination.offerId && !!flow.destination.offerUrl : !!flow.destination.url}
        previewUrl={flow.destination.kind === "offer" ? flow.destination.offerUrl : flow.destination.url}
        analytics={nodeAnalytics(flow.id, "destination")}
      >
        <div className="flex w-full flex-wrap items-center gap-2">
          <div className="flex overflow-hidden rounded-md border border-input">
            {(["offer", "redirect"] as const).map((kind) => (
              <button
                key={kind}
                type="button"
                onClick={() =>
                  onChange({
                    destination:
                      kind === "offer"
                        ? { kind: "offer", networkId: networks[0]?.id ?? "", offerId: "", offerUrl: "" }
                        : { kind: "redirect", url: "" },
                  })
                }
                className={cn(
                  "px-2 py-1 text-xs font-medium capitalize",
                  flow.destination.kind === kind
                    ? "bg-primary text-primary-foreground"
                    : "bg-transparent text-muted-foreground hover:bg-muted",
                )}
              >
                {kind}
              </button>
            ))}
          </div>

          {flow.destination.kind === "offer" ? (
            <>
              <Select
                value={flow.destination.networkId}
                onValueChange={(networkId) => {
                  const firstOffer = offers.find((o) => o.networkId === networkId);
                  onChange({
                    destination: {
                      kind: "offer",
                      networkId,
                      offerId: firstOffer?.id ?? "",
                      offerUrl: firstOffer?.links[0]?.url ?? "",
                    },
                  });
                }}
              >
                <SelectTrigger size="sm" className="w-36">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {networks.map((n) => (
                    <SelectItem key={n.id} value={n.id}>
                      {n.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Select
                value={flow.destination.offerId || undefined}
                onValueChange={(offerId) => {
                  const offer = offers.find((o) => o.id === offerId);
                  onChange({
                    destination:
                      flow.destination.kind === "offer"
                        ? { ...flow.destination, offerId, offerUrl: offer?.links[0]?.url ?? flow.destination.offerUrl }
                        : flow.destination,
                  });
                }}
              >
                <SelectTrigger size="sm" className="w-44">
                  <SelectValue placeholder="Choose offer" />
                </SelectTrigger>
                <SelectContent>
                  {networkOffers.map((o) => (
                    <SelectItem key={o.id} value={o.id}>
                      {o.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Input
                value={flow.destination.offerUrl}
                onChange={(e) =>
                  onChange({
                    destination:
                      flow.destination.kind === "offer" ? { ...flow.destination, offerUrl: e.target.value } : flow.destination,
                  })
                }
                placeholder="https://network.example/click?click_id={click_id}"
                className="h-7 min-w-56 flex-1 font-mono"
              />
            </>
          ) : (
            <Input
              value={flow.destination.url}
              onChange={(e) => onChange({ destination: { kind: "redirect", url: e.target.value } })}
              placeholder="https://example.com"
              className="h-7 min-w-56 flex-1 font-mono"
            />
          )}
        </div>
      </FlowNode>

      <Connector />

      <FlowNode
        icon={LifeBuoyIcon}
        label="Fallback"
        toggleable={false}
        enabled
        ghost
        configured={!!fallbackUrl}
        previewUrl={fallbackUrl || undefined}
      >
        <span className="text-xs text-muted-foreground">
          {fallbackUrl ? `Used if this flow can't be resolved: ${fallbackUrl}` : "No fallback set — the campaign fallback applies"}
        </span>
      </FlowNode>
    </div>
  );
}
