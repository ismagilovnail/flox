/** The `taggables` join side (§30.6) — which tags are on which entities.
 * Flows aren't seeded here: flow ids are generated per-campaign at runtime
 * (mock/stream-sets.ts), so there's nothing stable to hardcode against; the
 * tagging capability still works live, there's just no pre-seeded demo data
 * for that one entity type. */

import { generateCampaigns } from "@/lib/mock/campaigns";
import type { TaggableEntityType } from "@/lib/mock/tags";

export type TagAssignment = {
  tagId: string;
  entityType: TaggableEntityType;
  entityId: string;
};

const campaigns = generateCampaigns();

export const TAG_ASSIGNMENTS: TagAssignment[] = [
  { tagId: "tag_top_performer", entityType: "campaign", entityId: campaigns[0].id },
  { tagId: "tag_q3_push", entityType: "campaign", entityId: campaigns[0].id },
  { tagId: "tag_needs_review", entityType: "campaign", entityId: campaigns[1].id },
  { tagId: "tag_top_performer", entityType: "campaign", entityId: campaigns[5].id },
  { tagId: "tag_high_priority", entityType: "campaign", entityId: campaigns[2].id },

  { tagId: "tag_top_performer", entityType: "network", entityId: "net_afftrust" },
  { tagId: "tag_needs_review", entityType: "network", entityId: "net_direct" },

  { tagId: "tag_top_performer", entityType: "offer", entityId: "off_sweeps_us" },
  { tagId: "tag_q3_push", entityType: "offer", entityId: "off_sweeps_us" },
  { tagId: "tag_deprecated", entityType: "offer", entityId: "off_crypto_ca" },

  { tagId: "tag_high_priority", entityType: "traffic_source", entityId: "src_facebook" },
  { tagId: "tag_q3_push", entityType: "traffic_source", entityId: "src_tiktok" },

  { tagId: "tag_top_performer", entityType: "pwa", entityId: "pwa_sweeps" },

  { tagId: "tag_needs_review", entityType: "landing", entityId: "lnd_advertorial" },
];
