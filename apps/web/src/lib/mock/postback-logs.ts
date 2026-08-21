/**
 * Postback Logs (§29, §45) — every postback attempt, success/duplicate/error,
 * with replay ability. Two directions: `incoming` is a network calling
 * FLOX's own endpoint to report a conversion (dedup on (click_id, status)
 * applies here); `outgoing` is FLOX calling a Network's configured
 * postback URL to notify it of a status change.
 */

import { genId } from "@/lib/id";
import { generateConversions } from "@/lib/mock/conversions";
import type { CpaStatus } from "@/lib/api/conversions";
import { NETWORKS } from "@/lib/mock/networks";
import { EVENT_MAPPINGS } from "@/lib/mock/event-mappings";

function mulberry32(seed: number) {
  return function rand() {
    seed |= 0;
    seed = (seed + 0x6d2b79f5) | 0;
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

export type PostbackDirection = "incoming" | "outgoing";
export type PostbackResult = "success" | "duplicate" | "error";

export type PostbackLog = {
  id: string;
  direction: PostbackDirection;
  networkId: string;
  clickId: string;
  conversionId: string;
  rawStatus?: string;
  mappedStatus?: CpaStatus;
  revenue?: number;
  currency?: string;
  result: PostbackResult;
  message: string;
  createdAt: string;
};

export function generatePostbackLogs(): PostbackLog[] {
  const rand = mulberry32(55810);
  const conversions = generateConversions().slice(0, 24);
  const logs: PostbackLog[] = [];

  for (const conversion of conversions) {
    const network = NETWORKS.find((n) => n.id === conversion.networkId);
    const mapping = EVENT_MAPPINGS.find(
      (m) => m.networkId === conversion.networkId && m.floxStatus === conversion.status,
    );
    const incomingResult: PostbackResult = rand() > 0.9 ? "error" : rand() > 0.85 ? "duplicate" : "success";

    logs.push({
      id: genId(rand),
      direction: "incoming",
      networkId: conversion.networkId,
      clickId: conversion.clickId,
      conversionId: conversion.id,
      rawStatus: mapping?.networkStatus ?? conversion.status.toLowerCase(),
      mappedStatus: conversion.status,
      revenue: conversion.revenue,
      currency: conversion.currency,
      result: incomingResult,
      message:
        incomingResult === "success"
          ? "Conversion recorded"
          : incomingResult === "duplicate"
            ? "Dropped — duplicate on (click_id, status)"
            : "Failed to parse payload",
      createdAt: conversion.eventAt,
    });

    if (conversion.postbackStatus !== "not_configured") {
      const outgoingResult: PostbackResult = conversion.postbackStatus === "failed" ? "error" : "success";
      logs.push({
        id: genId(rand),
        direction: "outgoing",
        networkId: conversion.networkId,
        clickId: conversion.clickId,
        conversionId: conversion.id,
        revenue: conversion.revenue,
        currency: conversion.currency,
        result: outgoingResult,
        message:
          conversion.postbackStatus === "sent"
            ? `Delivered to ${network?.name ?? conversion.networkId}`
            : conversion.postbackStatus === "failed"
              ? "Delivery failed after 3 retries"
              : "Queued for delivery",
        createdAt: conversion.eventAt,
      });
    }
  }

  return logs.sort((a, b) => b.createdAt.localeCompare(a.createdAt));
}
