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

/** Landing/PWA/Postlanding stages and per-flow Pixels are gone here —
 * neither has a real backend yet (no internal/landing, internal/pwa,
 * internal/postlanding, or internal/pixel package exists), and their DB
 * columns/tables are nullable/optional specifically so a Flow can exist
 * without them. Dropped from the writable form rather than faked, same
 * precedent as every other domain this session. See
 * docs/stream-sets.md. */
const destinationSchema = z.union([
  z.object({ kind: z.literal("offer"), networkId: z.string(), offerId: z.string().min(1, "Choose an offer") }),
  z.object({ kind: z.literal("redirect"), url: z.url("Enter a valid URL") }),
]);

const flowSchema = z.object({
  id: z.string(),
  name: z.string().min(1, "Name is required"),
  active: z.boolean(),
  weight: z.number().min(0, "Weight can't be negative"),
  destination: destinationSchema,
});

export const streamSetFormSchema = z.object({
  name: z.string().min(2, "Name must be at least 2 characters").max(80),
  status: z.enum(["active", "paused"]),
  rootFilter: filterGroupSchema,
  flows: z.array(flowSchema).min(1, "At least one flow is required"),
  fallbackUrl: z.union([z.literal(""), z.url("Enter a valid URL")]),
});

export type StreamSetFormValues = z.infer<typeof streamSetFormSchema>;
