import { z } from "zod";

import {
  FILTER_FIELDS,
  FILTER_OPERATORS,
  OPERATORS_WITHOUT_VALUE,
  checkRE2Compatible,
  validateCountryValue,
  type FilterField,
  type FilterOperator,
} from "@/lib/filters";
import { PWA_TYPES, type PwaType } from "@/lib/mock/flow-entities";

const filterConditionSchema = z
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
      const error = validateCountryValue(condition.value);
      if (error) ctx.addIssue({ code: "custom", message: error, path: ["value"] });
    }

    if (condition.operator === "MATCHES") {
      const error = checkRE2Compatible(condition.value);
      if (error) ctx.addIssue({ code: "custom", message: error, path: ["value"] });
    }

    if (condition.operator === "BETWEEN" && (!condition.value || !condition.valueTo)) {
      ctx.addIssue({ code: "custom", message: "Enter both range bounds", path: ["valueTo"] });
    } else if (!condition.value) {
      ctx.addIssue({ code: "custom", message: "Value is required", path: ["value"] });
    }
  });

// Recursive node — z.lazy() needs an explicit TS type since z.infer can't
// derive a recursive type on its own.
export type FilterNodeFormValue =
  | z.infer<typeof filterConditionSchema>
  | { id: string; type: "group"; joiner: "AND" | "OR"; children: FilterNodeFormValue[] };

const filterGroupSchema: z.ZodType<FilterNodeFormValue & { type: "group" }> = z.lazy(() =>
  z.object({
    id: z.string(),
    type: z.literal("group"),
    joiner: z.enum(["AND", "OR"]),
    children: z.array(z.union([filterConditionSchema, filterGroupSchema])),
  }),
);

const landingStageSchema = z.object({ enabled: z.boolean(), landingId: z.string(), asPwa: z.boolean() });
const pwaStageSchema = z.object({
  enabled: z.boolean(),
  pwaId: z.string(),
  pwaType: z.enum(PWA_TYPES as [PwaType, ...PwaType[]]),
});
const postlandingStageSchema = z.object({ enabled: z.boolean(), postlandingId: z.string() });

const destinationSchema = z.union([
  z.object({ kind: z.literal("offer"), networkId: z.string(), offerId: z.string(), offerUrl: z.string() }),
  z.object({ kind: z.literal("redirect"), url: z.string() }),
]);

const flowSchema = z
  .object({
    id: z.string(),
    name: z.string().min(1, "Name is required"),
    active: z.boolean(),
    weight: z.number().min(0, "Weight can't be negative"),
    landing: landingStageSchema,
    pwa: pwaStageSchema,
    postlanding: postlandingStageSchema,
    destination: destinationSchema,
  })
  .superRefine((flow, ctx) => {
    if (flow.landing.enabled && !flow.landing.landingId) {
      ctx.addIssue({ code: "custom", message: "Choose a landing", path: ["landing", "landingId"] });
    }
    if (flow.pwa.enabled && !flow.pwa.pwaId) {
      ctx.addIssue({ code: "custom", message: "Choose a PWA", path: ["pwa", "pwaId"] });
    }
    if (flow.postlanding.enabled && !flow.postlanding.postlandingId) {
      ctx.addIssue({ code: "custom", message: "Choose a postlanding", path: ["postlanding", "postlandingId"] });
    }
    if (flow.destination.kind === "offer") {
      if (!flow.destination.offerId) {
        ctx.addIssue({ code: "custom", message: "Choose an offer", path: ["destination", "offerId"] });
      }
      if (!flow.destination.offerUrl || !/^https?:\/\//.test(flow.destination.offerUrl)) {
        ctx.addIssue({ code: "custom", message: "Enter a valid offer URL", path: ["destination", "offerUrl"] });
      }
    } else if (!flow.destination.url || !/^https?:\/\//.test(flow.destination.url)) {
      ctx.addIssue({ code: "custom", message: "Enter a valid redirect URL", path: ["destination", "url"] });
    }
  });

const pixelSchema = z.object({
  id: z.string(),
  url: z.url("Enter a valid URL"),
});

export const streamSetFormSchema = z.object({
  name: z.string().min(2, "Name must be at least 2 characters").max(80),
  status: z.enum(["active", "paused"]),
  rootFilter: filterGroupSchema,
  flows: z.array(flowSchema).min(1, "At least one flow is required"),
  pixels: z.array(pixelSchema),
  fallbackUrl: z.union([z.literal(""), z.url("Enter a valid URL")]),
});

export type StreamSetFormValues = z.infer<typeof streamSetFormSchema>;
