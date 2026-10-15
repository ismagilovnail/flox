import { apiFetch } from "@/lib/api/client";
import { fetchAllPages } from "@/lib/api/pagination";

/** Mirrors apps/internal/pixel's real shape. */
export type PixelStatus = "active" | "paused" | "archived";

export type PixelProvider = "facebook" | "tiktok" | "snapchat" | "generic";

/** pixel.Provider's values — every one sends. */
export const PIXEL_PROVIDERS: PixelProvider[] = ["facebook", "tiktok", "snapchat", "generic"];

/** Providers that authenticate with the pixel's access token (all but
 * generic, which calls its URL). */
export function providerUsesToken(p: PixelProvider): boolean {
  return p !== "generic";
}

/** Providers with a test event code (Snapchat has none). */
export function providerHasTestCode(p: PixelProvider): boolean {
  return p === "facebook" || p === "tiktok";
}

/** Display label i18n key per provider — the stored/wire value is always
 * the map key (raw, untranslated), same pattern as traffic-sources.ts's
 * SOURCE_TYPE_I18N_KEY. See docs/frontend-i18n.md. */
export const PIXEL_PROVIDER_I18N_KEY: Record<PixelProvider, string> = {
  facebook: "provider.facebook",
  tiktok: "provider.tiktok",
  snapchat: "provider.snapchat",
  generic: "provider.generic",
};

/**
 * The §43 events a pixel can report — matches pixel.ValidEventTypes
 * exactly: the events FLOX produces that an ad platform optimizes for.
 */
export const PIXEL_EVENT_TYPES = [
  "PWA_INSTALL",
  "NOTIFICATION_SUBSCRIBE",
  "TG_START",
  "TG_JOIN",
  "CPA_HOLD",
  "CPA_ACCEPT",
  "CPA_REDEP",
] as const;

export type PixelEventType = (typeof PIXEL_EVENT_TYPES)[number];

export type Pixel = {
  id: string;
  organizationId: string;
  name: string;
  provider: PixelProvider;
  pixelId: string;
  events: PixelEventType[];
  /** The token itself is never returned — only whether one is set and its
   * last 4 characters. */
  hasAccessToken: boolean;
  accessTokenLast4: string;
  testEventCode: string;
  /** Generic provider's GET URL template (macros). */
  url: string;
  /** Per-event override of the platform event name. */
  eventNames: Partial<Record<PixelEventType, string>>;
  status: PixelStatus;
  createdAt: string;
  updatedAt: string;
};

export type CreatePixelInput = {
  name: string;
  provider: PixelProvider;
  pixelId: string;
  events: PixelEventType[];
  accessToken?: string;
  testEventCode?: string;
  url?: string;
  eventNames?: Partial<Record<PixelEventType, string>>;
};

/** accessToken left out keeps the token; "" removes it. */
export type UpdatePixelInput = Partial<CreatePixelInput> & { status?: PixelStatus };

export type PixelDeliveryStatus = "queued" | "processing" | "success" | "retrying" | "dead";

/** One row of a pixel's delivery log (pixel_deliveries). */
export type PixelDelivery = {
  id: string;
  eventType: PixelEventType;
  clickId: string;
  status: PixelDeliveryStatus;
  attemptCount: number;
  responseStatusCode: number | null;
  message: string;
  createdAt: string;
  updatedAt: string;
};

/** The platform event each FLOX event is sent as by default, per provider
 * (pixel.DefaultEventNames) — the form's placeholders. */
export type PixelEventNameDefaults = Partial<Record<PixelProvider, Partial<Record<PixelEventType, string>>>>;

export function listPixels(): Promise<{ pixels: Pixel[]; total: number }> {
  return fetchAllPages<"pixels", Pixel>("/pixels", "pixels");
}

export function getPixel(id: string): Promise<Pixel> {
  return apiFetch(`/pixels/${id}`);
}

export function listPixelDeliveries(id: string): Promise<{ deliveries: PixelDelivery[] }> {
  return apiFetch(`/pixels/${id}/deliveries`);
}

export function getPixelEventNameDefaults(): Promise<{ defaults: PixelEventNameDefaults }> {
  return apiFetch("/pixels/event-names");
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

/** pixelsend.TestResult: how the platform answered a test event. */
export type PixelTestResult = {
  deliveryId: string;
  eventType: string;
  clickId: string;
  delivered: boolean;
  statusCode: number;
  message: string;
  /** The platform's test tool got it (a test event code is set), not the
   * ad account's statistics. */
  testMode: boolean;
  sentAt: string;
};

/** Facebook and TikTok with a test event code receive test events in their
 * test tool only; anywhere else the platform counts them as real
 * (pixelsend.InTestMode). */
export function pixelInTestMode(p: Pick<Pixel, "provider" | "testEventCode">): boolean {
  return (p.provider === "facebook" || p.provider === "tiktok") && p.testEventCode !== "";
}

/** Sends one test event now. `confirmReal` must be true when the platform
 * would count it (pixelInTestMode false), or the API refuses (422). */
export function sendPixelTestEvent(id: string, input: { eventType: string; confirmReal: boolean }): Promise<PixelTestResult> {
  return apiFetch(`/pixels/${id}/test-event`, { method: "POST", body: input });
}
