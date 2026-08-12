/**
 * Flow-level placeholder shared by the Flow Builder (§24-25).
 *
 * Networks, Offers, Landings, PWAs, and Postlandings are all real entities
 * now — see stores/{networks,offers,landings,pwas,postlandings}.ts (backed
 * by the matching mock/*.ts seed data). Flow Builder components read those
 * stores directly.
 *
 * `PwaType` is NOT one of those entities — it's the per-Flow display mode
 * for the PWA step (how *this* flow shows the PWA: as an internal page, an
 * external redirect, or an iOS app-store link), independent of which PWA
 * manifest is selected. It stays here.
 */

export type PwaType = "internal" | "external" | "ios_app";
export const PWA_TYPES: PwaType[] = ["internal", "external", "ios_app"];
