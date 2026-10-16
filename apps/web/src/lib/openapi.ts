/**
 * The public API reference's document (apps/internal/openapi/openapi.yaml,
 * served at GET /v1/openapi.json) and the pure helpers the reference page
 * reads it with. Only the parts of OpenAPI 3.1 that document uses are
 * typed here; the Go tests hold the document to that subset.
 */

export type JsonSchema = {
  $ref?: string;
  type?: string | string[];
  properties?: Record<string, JsonSchema>;
  required?: string[];
  additionalProperties?: boolean | JsonSchema;
  items?: JsonSchema;
  enum?: unknown[];
  anyOf?: JsonSchema[];
  format?: string;
  pattern?: string;
  description?: string;
  example?: unknown;
  default?: unknown;
  readOnly?: boolean;
  minimum?: number;
  maximum?: number;
  exclusiveMinimum?: number;
  minLength?: number;
  maxLength?: number;
  minItems?: number;
  maxItems?: number;
};

export type Parameter = {
  name: string;
  in: "path" | "query";
  required?: boolean;
  description?: string;
  schema?: JsonSchema;
};

type Ref = { $ref: string };

type MediaHolder = {
  description?: string;
  content?: Record<string, { schema?: JsonSchema }>;
};

export type Operation = {
  operationId: string;
  summary: string;
  description?: string;
  tags: string[];
  "x-flox-scope": "read" | "write";
  parameters?: (Parameter | Ref)[];
  requestBody?: MediaHolder & { required?: boolean };
  responses: Record<string, MediaHolder | Ref>;
};

export type HttpMethod = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

type PathItem = { parameters?: (Parameter | Ref)[] } & Partial<Record<Lowercase<HttpMethod>, Operation>>;

export type OpenApiDocument = {
  openapi: string;
  info: { title: string; version: string; summary?: string; description?: string };
  servers?: { url: string }[];
  tags?: { name: string; description?: string }[];
  paths: Record<string, PathItem>;
  components?: {
    schemas?: Record<string, JsonSchema>;
    parameters?: Record<string, Parameter>;
    responses?: Record<string, MediaHolder>;
  };
};

export type OperationEntry = {
  method: HttpMethod;
  path: string;
  operation: Operation;
  /** Path-level and operation-level parameters, references resolved. */
  parameters: Parameter[];
};

export type TagGroup = { name: string; description?: string; operations: OperationEntry[] };

const METHODS: HttpMethod[] = ["GET", "POST", "PUT", "PATCH", "DELETE"];

function isRef(x: unknown): x is Ref {
  return typeof x === "object" && x !== null && typeof (x as Ref).$ref === "string";
}

/** The component a local "#/components/…" reference points at. */
export function resolveRef<T = JsonSchema>(doc: OpenApiDocument, ref: string): T | undefined {
  if (!ref.startsWith("#/")) return undefined;
  let cur: unknown = doc;
  for (const part of ref.slice(2).split("/")) {
    if (typeof cur !== "object" || cur === null) return undefined;
    cur = (cur as Record<string, unknown>)[part];
  }
  return cur as T | undefined;
}

/** "#/components/schemas/Campaign" → "Campaign". */
export function refName(ref: string): string {
  return ref.slice(ref.lastIndexOf("/") + 1);
}

function resolveParameter(doc: OpenApiDocument, p: Parameter | Ref): Parameter | undefined {
  return isRef(p) ? resolveRef<Parameter>(doc, p.$ref) : p;
}

/** Every operation grouped by its tag, tags in the document's order. */
export function groupOperations(doc: OpenApiDocument): TagGroup[] {
  const groups = new Map<string, TagGroup>();
  for (const tag of doc.tags ?? []) groups.set(tag.name, { ...tag, operations: [] });
  for (const [path, item] of Object.entries(doc.paths)) {
    for (const method of METHODS) {
      const operation = item[method.toLowerCase() as Lowercase<HttpMethod>];
      if (!operation) continue;
      const parameters = [...(item.parameters ?? []), ...(operation.parameters ?? [])]
        .map((p) => resolveParameter(doc, p))
        .filter((p): p is Parameter => !!p);
      const tag = operation.tags[0] ?? "Other";
      if (!groups.has(tag)) groups.set(tag, { name: tag, operations: [] });
      groups.get(tag)!.operations.push({ method, path, operation, parameters });
    }
  }
  return [...groups.values()].filter((g) => g.operations.length > 0);
}

/** Groups narrowed to operations whose path, summary or id contains query. */
export function filterGroups(groups: TagGroup[], query: string): TagGroup[] {
  const q = query.trim().toLowerCase();
  if (!q) return groups;
  return groups
    .map((g) => ({
      ...g,
      operations: g.operations.filter(
        ({ method, path, operation }) =>
          `${method} ${path}`.toLowerCase().includes(q) ||
          operation.summary.toLowerCase().includes(q) ||
          operation.operationId.toLowerCase().includes(q) ||
          g.name.toLowerCase().includes(q),
      ),
    }))
    .filter((g) => g.operations.length > 0);
}

/** The operation's JSON request body schema. */
export function requestSchema(entry: OperationEntry): JsonSchema | undefined {
  return entry.operation.requestBody?.content?.["application/json"]?.schema;
}

/** The one 2xx response: its status, description and body schema (none for 204). */
export function successResponse(entry: OperationEntry): { status: string; description?: string; schema?: JsonSchema } | undefined {
  for (const [status, r] of Object.entries(entry.operation.responses)) {
    if (!status.startsWith("2") || isRef(r)) continue;
    return { status, description: r.description, schema: r.content?.["application/json"]?.schema };
  }
  return undefined;
}

function types(schema: JsonSchema): string[] {
  if (schema.type === undefined) return [];
  return Array.isArray(schema.type) ? schema.type : [schema.type];
}

/** A short type for a schema: "Campaign", "string · date-time",
 * "integer | null", "Campaign[]", "map of string", "Dimension | day". */
export function typeLabel(schema: JsonSchema): string {
  if (schema.$ref) return refName(schema.$ref);
  if (schema.anyOf) return schema.anyOf.map(typeLabel).join(" | ");
  const ts = types(schema);
  if (schema.enum && ts.length <= 1 && schema.enum.length === 1) return JSON.stringify(schema.enum[0]).replace(/"/g, "");
  const labels = ts.map((t) => {
    if (t === "array") return `${schema.items ? typeLabel(schema.items) : "any"}[]`;
    if (t === "object" && !schema.properties && typeof schema.additionalProperties === "object") {
      return `map of ${typeLabel(schema.additionalProperties)}`;
    }
    if (t === "string" && schema.format) return `string · ${schema.format}`;
    return t;
  });
  return labels.join(" | ") || "any";
}

/** The schema a property expands into, if it has fields of its own: an
 * object, an array of objects, or a reference. */
export function expandable(schema: JsonSchema): JsonSchema | undefined {
  if (schema.$ref || schema.properties) return schema;
  if (schema.items && (schema.items.$ref || schema.items.properties)) return schema.items;
  if (schema.anyOf) return schema.anyOf.find((s) => s.$ref || s.properties);
  return undefined;
}

/** Validation rules worth showing beside a field, e.g. "2–80 chars". */
export function constraints(schema: JsonSchema): string[] {
  const out: string[] = [];
  if (schema.minLength !== undefined && schema.maxLength !== undefined) out.push(`${schema.minLength}–${schema.maxLength} chars`);
  else if (schema.maxLength !== undefined) out.push(`≤ ${schema.maxLength} chars`);
  if (schema.minimum !== undefined && schema.maximum !== undefined) out.push(`${schema.minimum}–${schema.maximum}`);
  else if (schema.minimum !== undefined) out.push(`≥ ${schema.minimum}`);
  if (schema.exclusiveMinimum !== undefined) out.push(`> ${schema.exclusiveMinimum}`);
  if (schema.minItems !== undefined) out.push(`≥ ${schema.minItems} items`);
  if (schema.maxItems !== undefined) out.push(`≤ ${schema.maxItems} items`);
  if (schema.pattern) out.push(`/${schema.pattern}/`);
  if (schema.default !== undefined) out.push(`default ${JSON.stringify(schema.default)}`);
  return out;
}

const SAMPLE_DATE = "2026-10-01";
const MAX_SAMPLE_DEPTH = 12;

/** An example value for a schema: its own example, else something of its
 * type — required fields only for objects, so the example is the smallest
 * valid body. */
export function sampleValue(doc: OpenApiDocument, schema: JsonSchema, depth = 0): unknown {
  if (schema.example !== undefined) return schema.example;
  if (schema.$ref) {
    const target = resolveRef(doc, schema.$ref);
    return target && depth < MAX_SAMPLE_DEPTH ? sampleValue(doc, target, depth + 1) : {};
  }
  if (schema.anyOf?.length) return sampleValue(doc, schema.anyOf[0], depth + 1);
  if (schema.enum?.length) return schema.enum[0];
  const t = types(schema).find((x) => x !== "null");
  switch (t) {
    case "string":
      if (schema.format === "date") return SAMPLE_DATE;
      if (schema.format === "date-time") return `${SAMPLE_DATE}T12:00:00Z`;
      if (schema.format === "uri") return "https://example.com/";
      return "string";
    case "integer":
      return schema.minimum ?? 1;
    case "number":
      return schema.minimum ?? 10.5;
    case "boolean":
      return true;
    case "array":
      return schema.items && depth < MAX_SAMPLE_DEPTH ? [sampleValue(doc, schema.items, depth + 1)] : [];
    case "object": {
      const out: Record<string, unknown> = {};
      for (const name of schema.required ?? []) {
        const prop = schema.properties?.[name];
        if (prop) out[name] = depth < MAX_SAMPLE_DEPTH ? sampleValue(doc, prop, depth + 1) : null;
      }
      return out;
    }
  }
  return null;
}

/** A ready curl call for the operation, with path parameters left as
 * placeholders and the key read from $FLOX_API_KEY. */
export function curlExample(doc: OpenApiDocument, serverUrl: string, entry: OperationEntry): string {
  const path = entry.path.replace(/\{([^}]+)\}/g, (_, name: string) => `<${name}>`);
  const lines = [`curl -X ${entry.method} "${serverUrl.replace(/\/$/, "")}${path}"`, `  -H "Authorization: Bearer $FLOX_API_KEY"`];
  const body = requestSchema(entry);
  if (body) {
    lines.push(`  -H "Content-Type: application/json"`);
    lines.push(`  -d '${JSON.stringify(sampleValue(doc, body), null, 2).replace(/\n/g, "\n  ")}'`);
  }
  return lines.join(" \\\n");
}
