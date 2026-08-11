import { z } from "zod";

import {
  FILTER_FIELDS,
  FILTER_OPERATORS,
  FLOW_DESTINATION_TYPES,
  type FilterField,
  type FilterOperator,
  type FlowDestinationType,
} from "@/lib/mock/stream-sets";

const filterConditionSchema = z.object({
  id: z.string(),
  field: z.enum(FILTER_FIELDS as [FilterField, ...FilterField[]]),
  operator: z.enum(FILTER_OPERATORS as [FilterOperator, ...FilterOperator[]]),
  value: z.string(),
});

const flowSchema = z.object({
  id: z.string(),
  name: z.string().min(1, "Name is required"),
  destinationType: z.enum(FLOW_DESTINATION_TYPES as [FlowDestinationType, ...FlowDestinationType[]]),
  destinationUrl: z.url("Enter a valid URL"),
  weight: z.number().min(0, "0-100").max(100, "0-100"),
  active: z.boolean(),
});

const pixelSchema = z.object({
  id: z.string(),
  url: z.url("Enter a valid URL"),
});

export const streamSetFormSchema = z.object({
  name: z.string().min(2, "Name must be at least 2 characters").max(80),
  status: z.enum(["active", "paused"]),
  joiner: z.enum(["AND", "OR"]),
  filters: z.array(filterConditionSchema),
  flows: z.array(flowSchema).min(1, "At least one flow is required"),
  pixels: z.array(pixelSchema),
  fallbackUrl: z.union([z.literal(""), z.url("Enter a valid URL")]),
});

export type StreamSetFormValues = z.infer<typeof streamSetFormSchema>;
