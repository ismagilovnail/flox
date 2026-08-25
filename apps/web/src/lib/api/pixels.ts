import { apiFetch } from "@/lib/api/client";

/** Mirrors apps/internal/pixel's real shape. */
export type PixelStatus = "active" | "paused" | "archived";

export type PixelProvider = "facebook" | "tiktok" | "snapchat" | "twitter" | "generic";

export const PIXEL_PROVIDERS: PixelProvider[] = ["facebook", "tiktok", "snapchat", "twitter", "generic"];

/** Display label i18n key per provider — the stored/wire value is always
 * the map key (raw, untranslated), same pattern as traffic-sources.ts's
 * SOURCE_TYPE_I18N_KEY. See docs/frontend-i18n.md. */
export const PIXEL_PROVIDER_I18N_KEY: Record<PixelProvider, string> = {
  facebook: "provider.facebook",
  tiktok: "provider.tiktok",
  snapchat: "provider.snapchat",
  twitter: "provider.twitter",
  generic: "provider.generic",
};

/**
 * Curated subset of the full §43 event model a conversion pixel plausibly
 * fires on — matches pixel.ValidEventTypes exactly. The full canonical
 * event enum belongs to the Conversions/Postbacks domain; don't duplicate
 * it here, just reference the same string values.
 */
export const PIXEL_EVENT_TYPES = [
  "PWA_INSTALL",
  "CPA_HOLD",
  "CPA_ACCEPT",
  "CPA_REDEP",
  "TG_JOIN",
  "NOTIFICATION_SUBSCRIBE",
] as const;

export type PixelEventType = (typeof PIXEL_EVENT_TYPES)[number];

export type Pixel = {
  id: string;
  organizationId: string;
  name: string;
  provider: PixelProvider;
  pixelId: string;
  events: PixelEventType[];
  status: PixelStatus;
  createdAt: string;
  updatedAt: string;
};

export type CreatePixelInput = {
  name: string;
  provider: PixelProvider;
  pixelId: string;
  events: PixelEventType[];
};

export type UpdatePixelInput = Partial<CreatePixelInput> & { status?: PixelStatus };

export function listPixels(): Promise<{ pixels: Pixel[] }> {
  return apiFetch("/pixels");
}

export function getPixel(id: string): Promise<Pixel> {
  return apiFetch(`/pixels/${id}`);
}

export function createPixel(input: CreatePixelInput): Promise<Pixel> {
  return apiFetch("/pixels", { method: "POST", body: input });
}

export function updatePixel(id: string, input: UpdatePixelInput): Promise<Pixel> {
  return apiFetch(`/pixels/${id}`, { method: "PATCH", body: input });
}

export function deletePixel(id: string): Promise<void> {
  return apiFetch(`/pixels/${id}`, { method: "DELETE" });
}

export function duplicatePixel(id: string): Promise<Pixel> {
  return apiFetch(`/pixels/${id}/duplicate`, { method: "POST" });
}

export function pausePixel(id: string): Promise<Pixel> {
  return apiFetch(`/pixels/${id}/pause`, { method: "POST" });
}

export function activatePixel(id: string): Promise<Pixel> {
  return apiFetch(`/pixels/${id}/activate`, { method: "POST" });
}

export function archivePixel(id: string): Promise<Pixel> {
  return apiFetch(`/pixels/${id}`, { method: "PATCH", body: { status: "archived" } });
}
