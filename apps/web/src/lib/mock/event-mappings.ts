/**
 * Event Mapping (§29) — per-network translation from that network's own raw
 * status string (whatever they send on their incoming postback call) to the
 * canonical FLOX CpaStatus enum (§43). This is what the Conversion Engine
 * (Phase 23) runs at ingest time; here it's the team-editable config surface.
 */

import type { CpaStatus } from "@/lib/mock/conversions";

export type EventMapping = {
  id: string;
  networkId: string;
  networkStatus: string;
  floxStatus: CpaStatus;
};

export const EVENT_MAPPINGS: EventMapping[] = [
  { id: "map_afftrust_lead", networkId: "net_afftrust", networkStatus: "lead", floxStatus: "CPA_HOLD" },
  { id: "map_afftrust_sale", networkId: "net_afftrust", networkStatus: "sale", floxStatus: "CPA_ACCEPT" },
  { id: "map_afftrust_rebill", networkId: "net_afftrust", networkStatus: "rebill", floxStatus: "CPA_REDEP" },
  { id: "map_afftrust_reject", networkId: "net_afftrust", networkStatus: "reject", floxStatus: "CPA_DECLINE" },

  { id: "map_adcombo_reg", networkId: "net_adcombo", networkStatus: "reg", floxStatus: "CPA_HOLD" },
  { id: "map_adcombo_ftd", networkId: "net_adcombo", networkStatus: "ftd", floxStatus: "CPA_ACCEPT" },
  { id: "map_adcombo_chargeback", networkId: "net_adcombo", networkStatus: "chargeback", floxStatus: "CPA_TRASH" },

  { id: "map_mylead_pending", networkId: "net_mylead", networkStatus: "pending", floxStatus: "CPA_HOLD" },
  { id: "map_mylead_confirmed", networkId: "net_mylead", networkStatus: "confirmed", floxStatus: "CPA_ACCEPT" },
  { id: "map_mylead_declined", networkId: "net_mylead", networkStatus: "declined", floxStatus: "CPA_DECLINE" },

  { id: "map_direct_deposit", networkId: "net_direct", networkStatus: "deposit", floxStatus: "CPA_ACCEPT" },
  { id: "map_direct_redeposit", networkId: "net_direct", networkStatus: "redeposit", floxStatus: "CPA_REDEP" },
];
