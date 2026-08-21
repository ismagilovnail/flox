/**
 * Conversions mock — kept alive only for `generateConversions`, which the
 * still-mocked Postback Logs feature (lib/mock/postback-logs.ts)
 * cross-references to synthesize fake postback attempts. The real
 * Conversions list/detail/timeline (§29, §43) is wired to the real
 * backend now — see lib/api/conversions.ts, which is also the real home
 * for CpaStatus/CPA_STATUSES (a real domain enum, not mock-specific,
 * imported from there directly by every consumer, this file included).
 */

import { genId } from "@/lib/id";
import { generateCampaigns } from "@/lib/mock/campaigns";
import { OFFERS } from "@/lib/mock/offers";
import type { CpaStatus } from "@/lib/api/conversions";

function mulberry32(seed: number) {
  return function rand() {
    seed |= 0;
    seed = (seed + 0x6d2b79f5) | 0;
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function round2(n: number) {
  return Math.round(n * 100) / 100;
}

export type PostbackDeliveryStatus = "sent" | "pending" | "failed" | "not_configured";

export type Conversion = {
  id: string;
  clickId: string;
  campaignId: string;
  offerId: string;
  networkId: string;
  status: CpaStatus;
  /** Original-currency revenue at event time (§50-FX: never re-priced with a later rate). */
  revenue: number;
  currency: string;
  eventAt: string;
  postbackStatus: PostbackDeliveryStatus;
};

const STATUS_CYCLE: CpaStatus[] = [
  "CPA_HOLD",
  "CPA_HOLD",
  "CPA_ACCEPT",
  "CPA_ACCEPT",
  "CPA_ACCEPT",
  "CPA_REDEP",
  "CPA_REDEP",
  "CPA_DECLINE",
  "CPA_TRASH",
];

/** Deterministic mock conversion feed cross-referencing the real Campaigns/Offers seed data. */
export function generateConversions(count = 42): Conversion[] {
  const rand = mulberry32(90210);
  const campaigns = generateCampaigns();
  const today = new Date("2026-08-11T00:00:00Z");

  const conversions = Array.from({ length: count }, () => {
    const offer = OFFERS[Math.floor(rand() * OFFERS.length)];
    const campaign = campaigns[Math.floor(rand() * campaigns.length)];
    const status = STATUS_CYCLE[Math.floor(rand() * STATUS_CYCLE.length)];
    const minutesAgo = Math.floor(rand() * 60 * 24 * 45);
    const eventAt = new Date(today);
    eventAt.setUTCMinutes(eventAt.getUTCMinutes() - minutesAgo);

    const isPayable = status === "CPA_ACCEPT" || status === "CPA_REDEP" || status === "CPA_HOLD";
    const revenue = isPayable ? round2(offer.payout * (0.85 + rand() * 0.3)) : 0;

    const postbackStatus: PostbackDeliveryStatus =
      status === "CPA_DECLINE" || status === "CPA_TRASH"
        ? "not_configured"
        : rand() > 0.15
          ? "sent"
          : rand() > 0.5
            ? "pending"
            : "failed";

    return {
      id: genId(rand),
      clickId: genId(rand),
      campaignId: campaign.id,
      offerId: offer.id,
      networkId: offer.networkId,
      status,
      revenue,
      currency: offer.currency,
      eventAt: eventAt.toISOString(),
      postbackStatus,
    };
  });

  return conversions.sort((a, b) => b.eventAt.localeCompare(a.eventAt));
}

export type TimelineStage = "Click" | "Landing" | "PWA" | "Offer" | "Conversion" | "Postback";

export type TimelineStep = {
  stage: TimelineStage;
  label: string;
  timestamp: string;
  description: string;
};

const STATUS_LABEL: Record<CpaStatus, string> = {
  CPA_HOLD: "Registration held",
  CPA_ACCEPT: "First deposit accepted",
  CPA_REDEP: "Re-deposit accepted",
  CPA_DECLINE: "Conversion declined",
  CPA_TRASH: "Marked junk/duplicate",
};

const POSTBACK_LABEL: Record<PostbackDeliveryStatus, string> = {
  sent: "Delivered to network",
  pending: "Queued for delivery",
  failed: "Delivery failed — will retry",
  not_configured: "No postback required for this status",
};

/** Fixed 6-stage funnel per §29 — every conversion detail shows the same shape,
 * regardless of which optional flow stages (landing/PWA) that campaign uses. */
export function generateConversionTimeline(conversion: Conversion): TimelineStep[] {
  const base = new Date(conversion.eventAt);
  function offsetMinutes(minutes: number) {
    const d = new Date(base);
    d.setUTCMinutes(d.getUTCMinutes() - minutes);
    return d.toISOString();
  }

  return [
    { stage: "Click", label: "Source click", timestamp: offsetMinutes(38), description: `click_id ${conversion.clickId}` },
    { stage: "Landing", label: "Landing viewed", timestamp: offsetMinutes(35), description: "Landing page rendered" },
    { stage: "PWA", label: "PWA installed", timestamp: offsetMinutes(20), description: "Install prompt accepted" },
    { stage: "Offer", label: "Offer opened", timestamp: offsetMinutes(9), description: "Redirected to offer link" },
    {
      stage: "Conversion",
      label: STATUS_LABEL[conversion.status],
      timestamp: conversion.eventAt,
      description: `${conversion.revenue.toFixed(2)} ${conversion.currency}`,
    },
    {
      stage: "Postback",
      label: POSTBACK_LABEL[conversion.postbackStatus],
      timestamp: offsetMinutes(-1),
      description: conversion.postbackStatus === "not_configured" ? "" : "Outgoing postback to network",
    },
  ];
}
