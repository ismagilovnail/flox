import { z } from "zod";
import type { TFunction } from "i18next";

import {
  FILTER_FIELDS,
  FILTER_OPERATORS,
  OPERATORS_WITHOUT_VALUE,
  checkRE2Compatible,
  validateCountryValue,
  type FilterField,
  type FilterOperator,
} from "@/lib/filters";

function buildFilterConditionSchema(t: TFunction) {
  return z
    .object({
      id: z.string(),
      type: z.literal("condition"),
      field: z.enum(FILTER_FIELDS as [FilterField, ...FilterField[]]),
      operator: z.enum(FILTER_OPERATORS as [FilterOperator, ...FilterOperator[]]),
      value: z.string(),
      valueTo: z.string(),
    })
    .superRefine((condition, ctx) => {
      if (OPERATORS_WITHOUT_VALUE.includes(condition.operator)) return;

      if (condition.field === "country" && condition.value) {
        const error = validateCountryValue(condition.value, t);
        if (error) ctx.addIssue({ code: "custom", message: error, path: ["value"] });
      }

      if (condition.operator === "MATCHES") {
        const error = checkRE2Compatible(condition.value, t);
        if (error) ctx.addIssue({ code: "custom", message: error, path: ["value"] });
      }

      if (condition.operator === "BETWEEN" && (!condition.value || !condition.valueTo)) {
        ctx.addIssue({ code: "custom", message: t("form.validation.bothRangeBounds", { ns: "streamSets" }), path: ["valueTo"] });
      } else if (!condition.value) {
        ctx.addIssue({ code: "custom", message: t("form.validation.chooseValue", { ns: "streamSets" }), path: ["value"] });
      }
    });
}

// Recursive node — z.lazy() needs an explicit TS type since z.infer can't
// derive a recursive type on its own.
export type FilterNodeFormValue =
  | { id: string; type: "condition"; field: FilterField; operator: FilterOperator; value: string; valueTo: string }
  | { id: string; type: "group"; joiner: "AND" | "OR"; children: FilterNodeFormValue[] };

function buildFilterGroupSchema(t: TFunction): z.ZodType<FilterNodeFormValue & { type: "group" }> {
  const filterConditionSchema = buildFilterConditionSchema(t);
  const filterGroupSchema: z.ZodType<FilterNodeFormValue & { type: "group" }> = z.lazy(() =>
    z.object({
      id: z.string(),
      type: z.literal("group"),
      joiner: z.enum(["AND", "OR"]),
      children: z.array(z.union([filterConditionSchema, filterGroupSchema])),
    }),
  );
  return filterGroupSchema;
}

function buildDestinationSchema(t: TFunction) {
  return z.union([
    z.object({
      kind: z.literal("offer"),
      networkId: z.string(),
      offerId: z.string().min(1, t("form.validation.chooseOffer", { ns: "streamSets" })),
    }),
    z.object({ kind: z.literal("redirect"), url: z.url(t("form.validation.urlInvalid", { ns: "streamSets" })) }),
  ]);
}

function buildFlowLandingSchema(t: TFunction) {
  return z
    .object({ enabled: z.boolean(), landingId: z.string(), asPwa: z.boolean() })
    .superRefine((landing, ctx) => {
      if (landing.enabled && !landing.landingId) {
        ctx.addIssue({ code: "custom", message: t("form.validation.chooseLanding", { ns: "streamSets" }), path: ["landingId"] });
      }
    });
}

function buildFlowPwaSchema(t: TFunction) {
  return z
    .object({ enabled: z.boolean(), pwaId: z.string(), pwaType: z.enum(["internal", "external", "ios_app", ""]) })
    .superRefine((pwa, ctx) => {
      if (!pwa.enabled) return;
      if (!pwa.pwaId) {
        ctx.addIssue({ code: "custom", message: t("form.validation.choosePwa", { ns: "streamSets" }), path: ["pwaId"] });
      }
      if (!pwa.pwaType) {
        ctx.addIssue({ code: "custom", message: t("form.validation.choosePwaType", { ns: "streamSets" }), path: ["pwaType"] });
      }
    });
}

function buildFlowPostlandingSchema(t: TFunction) {
  return z
    .object({ enabled: z.boolean(), postlandingId: z.string() })
    .superRefine((postlanding, ctx) => {
      if (postlanding.enabled && !postlanding.postlandingId) {
        ctx.addIssue({ code: "custom", message: t("form.validation.choosePostlanding", { ns: "streamSets" }), path: ["postlandingId"] });
      }
    });
}

function buildFlowSchema(t: TFunction) {
  return z.object({
    id: z.string(),
    name: z.string().min(1, t("form.validation.flowNameRequired", { ns: "streamSets" })),
    active: z.boolean(),
    weight: z.number().min(0, t("form.validation.weightNegative", { ns: "streamSets" })),
    landing: buildFlowLandingSchema(t),
    pwa: buildFlowPwaSchema(t),
    postlanding: buildFlowPostlandingSchema(t),
    destination: buildDestinationSchema(t),
  });
}

/** Factory, not a module-level const — see source-form-sheet.tsx's
 * buildSourceFormSchema for why (Zod messages are user-facing text and
 * need the live translator). */
export function buildStreamSetFormSchema(t: TFunction) {
  return z.object({
    name: z.string().min(2, t("form.validation.nameMin", { ns: "streamSets" })).max(80),
    status: z.enum(["active", "paused"]),
    rootFilter: buildFilterGroupSchema(t),
    flows: z.array(buildFlowSchema(t)).min(1, t("form.validation.atLeastOneFlow", { ns: "streamSets" })),
    // A Stream-Set-level attachment (stream_set_pixels), not a per-Flow
    // concern — zero pixels is a valid, common configuration (an operator
    // may not want any ad-platform pixel firing for this set), so no
    // .min(1) here, unlike flows.
    pixelIds: z.array(z.string()),
    fallbackUrl: z.union([z.literal(""), z.url(t("form.validation.urlInvalid", { ns: "streamSets" }))]),
  });
}

export type StreamSetFormValues = z.infer<ReturnType<typeof buildStreamSetFormSchema>>;
