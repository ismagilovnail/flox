"use client";

import { ArrowUpRightIcon, LifeBuoyIcon, TargetIcon } from "lucide-react";

import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { cn } from "@/lib/utils";
import type { Network } from "@/lib/api/networks";
import type { Offer } from "@/lib/api/offers";
import type { StreamSetFormValues } from "@/features/stream-sets/stream-set-schema";
import { FlowNode } from "@/features/stream-sets/flow-node";

type FlowFormValue = StreamSetFormValues["flows"][number];

// Radix Select's controlled `value` prop behaves unreliably once it's ever
// been an empty string — selecting an item afterward silently fails to
// update the trigger (reproduced live: onValueChange never fires again
// after a render with value=""). Radix's own docs warn against an empty
// SelectItem value for the same underlying reason. NO_OFFER stands in for
// "nothing chosen yet" on the wire so the Select is never handed "".
const NO_OFFER = "__no_offer__";

function Connector() {
  return <div className="ml-[15px] h-3 w-px bg-border" />;
}

/** Replaces FlowFunnel's Landing/PWA/Postlanding/Offer/Redirect/Fallback
 * chain with just Offer-or-Redirect + Fallback — the three dropped stages
 * have no backend (no internal/landing, internal/pwa, or
 * internal/postlanding package exists yet), so there's nothing real for
 * their pickers to select from. See docs/stream-sets.md. */
export function FlowDestinationEditor({
  flow,
  fallbackUrl,
  networks,
  offers,
  onChange,
}: {
  flow: FlowFormValue;
  fallbackUrl: string;
  networks: Network[];
  offers: Offer[];
  onChange: (patch: Partial<FlowFormValue>) => void;
}) {
  const destination = flow.destination;
  const networkOffers = destination.kind === "offer" ? offers.filter((o) => o.networkId === destination.networkId) : [];

  return (
    <div className="flex flex-col">
      <FlowNode
        icon={destination.kind === "offer" ? TargetIcon : ArrowUpRightIcon}
        label={destination.kind === "offer" ? "Offer" : "Redirect"}
        toggleable={false}
        enabled
        configured={destination.kind === "offer" ? !!destination.offerId : !!destination.url}
      >
        <div className="flex w-full flex-wrap items-center gap-2">
          <div className="flex overflow-hidden rounded-md border border-input">
            {(["offer", "redirect"] as const).map((kind) => (
              <button
                key={kind}
                type="button"
                onClick={() =>
                  onChange({
                    destination: kind === "offer" ? { kind: "offer", networkId: networks[0]?.id ?? "", offerId: "" } : { kind: "redirect", url: "" },
                  })
                }
                className={cn(
                  "px-2 py-1 text-xs font-medium capitalize",
                  destination.kind === kind
                    ? "bg-primary text-primary-foreground"
                    : "bg-transparent text-muted-foreground hover:bg-muted",
                )}
              >
                {kind}
              </button>
            ))}
          </div>

          {destination.kind === "offer" ? (
            <>
              <Select
                value={destination.networkId}
                onValueChange={(networkId) => {
                  const firstOffer = offers.find((o) => o.networkId === networkId);
                  onChange({ destination: { kind: "offer", networkId, offerId: firstOffer?.id ?? "" } });
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
                value={destination.offerId || NO_OFFER}
                onValueChange={(offerId) => onChange({ destination: { kind: "offer", networkId: destination.networkId, offerId: offerId === NO_OFFER ? "" : offerId } })}
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
            </>
          ) : (
            <Input
              value={destination.url}
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
