import { genId } from "@/lib/id";

export type FilterField =
  | "country"
  | "region"
  | "city"
  | "device"
  | "platform"
  | "os"
  | "os_version"
  | "browser"
  | "browser_version"
  | "language"
  | "bot"
  | "proxy"
  | "asn"
  | "connection_type"
  | "referrer"
  | "utm_source"
  | "utm_medium"
  | "utm_campaign"
  | "utm_content"
  | "utm_term"
  | "sub1"
  | "sub2"
  | "sub3"
  | "sub4"
  | "sub5"
  | "sub6"
  | "sub7"
  | "sub8"
  | "sub9"
  | "sub10"
  | "external_click_id";

export const FIELD_GROUPS: { label: string; fields: FilterField[] }[] = [
  { label: "Geo", fields: ["country", "region", "city"] },
  {
    label: "Device",
    fields: ["device", "platform", "os", "os_version", "browser", "browser_version", "language"],
  },
  { label: "Fraud", fields: ["bot", "proxy", "asn", "connection_type"] },
  {
    label: "Traffic",
    fields: ["referrer", "utm_source", "utm_medium", "utm_campaign", "utm_content", "utm_term"],
  },
  {
    label: "Custom",
    fields: [
      "sub1", "sub2", "sub3", "sub4", "sub5", "sub6", "sub7", "sub8", "sub9", "sub10",
      "external_click_id",
    ],
  },
];

export const FILTER_FIELDS: FilterField[] = FIELD_GROUPS.flatMap((g) => g.fields);

export type FilterOperator =
  | "IS"
  | "IS_NOT"
  | "IN"
  | "NOT_IN"
  | "CONTAINS"
  | "NOT_CONTAINS"
  | "STARTS_WITH"
  | "ENDS_WITH"
  | "MATCHES"
  | "EXISTS"
  | "NOT_EXISTS"
  | "GT"
  | "GTE"
  | "LT"
  | "LTE"
  | "BETWEEN";

export const FILTER_OPERATORS: FilterOperator[] = [
  "IS", "IS_NOT", "IN", "NOT_IN",
  "CONTAINS", "NOT_CONTAINS", "STARTS_WITH", "ENDS_WITH", "MATCHES",
  "EXISTS", "NOT_EXISTS",
  "GT", "GTE", "LT", "LTE", "BETWEEN",
];

/** No value input for existence checks — the field alone is the condition. */
export const OPERATORS_WITHOUT_VALUE: FilterOperator[] = ["EXISTS", "NOT_EXISTS"];
export const MULTI_VALUE_OPERATORS: FilterOperator[] = ["IN", "NOT_IN"];
export const RANGE_OPERATORS: FilterOperator[] = ["BETWEEN"];

export const BOOLEAN_FLAG_FIELDS: FilterField[] = ["bot", "proxy"];

/** Fields with a bounded vocabulary — get a picker instead of free text. */
export const FIELD_VOCAB: Partial<Record<FilterField, { value: string; label: string }[]>> = {
  device: [
    { value: "mobile", label: "Mobile" },
    { value: "desktop", label: "Desktop" },
    { value: "tablet", label: "Tablet" },
  ],
  platform: [
    { value: "ios", label: "iOS" },
    { value: "android", label: "Android" },
    { value: "windows", label: "Windows" },
    { value: "macos", label: "macOS" },
    { value: "linux", label: "Linux" },
  ],
  os: [
    { value: "ios", label: "iOS" },
    { value: "android", label: "Android" },
    { value: "windows", label: "Windows" },
    { value: "macos", label: "macOS" },
    { value: "linux", label: "Linux" },
  ],
  browser: [
    { value: "chrome", label: "Chrome" },
    { value: "safari", label: "Safari" },
    { value: "firefox", label: "Firefox" },
    { value: "edge", label: "Edge" },
    { value: "samsung_internet", label: "Samsung Internet" },
    { value: "other", label: "Other" },
  ],
  connection_type: [
    { value: "wifi", label: "Wi-Fi" },
    { value: "cellular", label: "Cellular" },
    { value: "ethernet", label: "Ethernet" },
    { value: "unknown", label: "Unknown" },
  ],
};

export type FilterCondition = {
  id: string;
  type: "condition";
  field: FilterField;
  operator: FilterOperator;
  value: string;
  valueTo: string;
};

export type FilterGroupNode = {
  id: string;
  type: "group";
  joiner: "AND" | "OR";
  children: FilterNode[];
};

export type FilterNode = FilterCondition | FilterGroupNode;

export function newCondition(): FilterCondition {
  return { id: genId(), type: "condition", field: "country", operator: "IS", value: "", valueTo: "" };
}

export function emptyGroup(joiner: "AND" | "OR" = "AND"): FilterGroupNode {
  return { id: genId(), type: "group", joiner, children: [] };
}

function mapChildren(group: FilterGroupNode, fn: (children: FilterNode[]) => FilterNode[]): FilterGroupNode {
  return { ...group, children: fn(group.children) };
}

function walk(group: FilterGroupNode, transform: (node: FilterNode) => FilterNode | null): FilterGroupNode {
  return mapChildren(group, (children) =>
    children
      .map((child) => (child.type === "group" ? walk(child, transform) : child))
      .map((child) => transform(child))
      .filter((child): child is FilterNode => child !== null),
  );
}

export function addConditionToGroup(root: FilterGroupNode, groupId: string): FilterGroupNode {
  if (root.id === groupId) return mapChildren(root, (children) => [...children, newCondition()]);
  return mapChildren(root, (children) =>
    children.map((child) => (child.type === "group" ? addConditionToGroup(child, groupId) : child)),
  );
}

export function addGroupToGroup(root: FilterGroupNode, groupId: string): FilterGroupNode {
  const newGroup = emptyGroup(root.id === groupId ? (root.joiner === "AND" ? "OR" : "AND") : "AND");
  if (root.id === groupId) return mapChildren(root, (children) => [...children, newGroup]);
  return mapChildren(root, (children) =>
    children.map((child) => (child.type === "group" ? addGroupToGroup(child, groupId) : child)),
  );
}

export function updateCondition(root: FilterGroupNode, id: string, patch: Partial<FilterCondition>): FilterGroupNode {
  return walk(root, (node) => (node.type === "condition" && node.id === id ? { ...node, ...patch } : node));
}

export function updateGroupJoiner(root: FilterGroupNode, groupId: string, joiner: "AND" | "OR"): FilterGroupNode {
  if (root.id === groupId) return { ...root, joiner };
  return mapChildren(root, (children) =>
    children.map((child) => (child.type === "group" ? updateGroupJoiner(child, groupId, joiner) : child)),
  );
}

export function removeNode(root: FilterGroupNode, nodeId: string): FilterGroupNode {
  return walk(root, (node) => (node.id === nodeId ? null : node));
}

export function cloneWithNewIds(node: FilterNode): FilterNode {
  if (node.type === "condition") return { ...node, id: genId() };
  return { ...node, id: genId(), children: node.children.map(cloneWithNewIds) };
}

export function countConditions(node: FilterNode): number {
  if (node.type === "condition") return 1;
  return node.children.reduce((sum, child) => sum + countConditions(child), 0);
}

const OPERATOR_LABEL: Record<FilterOperator, string> = {
  IS: "is",
  IS_NOT: "is not",
  IN: "in",
  NOT_IN: "not in",
  CONTAINS: "contains",
  NOT_CONTAINS: "does not contain",
  STARTS_WITH: "starts with",
  ENDS_WITH: "ends with",
  MATCHES: "matches",
  EXISTS: "exists",
  NOT_EXISTS: "does not exist",
  GT: ">",
  GTE: ">=",
  LT: "<",
  LTE: "<=",
  BETWEEN: "between",
};

function describeCondition(c: FilterCondition): string {
  if (OPERATORS_WITHOUT_VALUE.includes(c.operator)) return `${c.field} ${OPERATOR_LABEL[c.operator]}`;
  if (c.operator === "BETWEEN") return `${c.field} ${OPERATOR_LABEL[c.operator]} [${c.value}, ${c.valueTo}]`;
  return `${c.field} ${OPERATOR_LABEL[c.operator]} ${c.value || "—"}`;
}

/** Plain-language rendering of the whole tree — the static explainability
 * surface for this phase (the interactive "why did this match" view is the
 * Phase 10 simulator). */
export function describeFilterTree(node: FilterNode): string {
  if (node.type === "condition") return describeCondition(node);
  if (node.children.length === 0) return "always";
  const parts = node.children.map((child) =>
    child.type === "group" && child.children.length > 1 ? `(${describeFilterTree(child)})` : describeFilterTree(child),
  );
  return parts.join(` ${node.joiner} `);
}

const ISO_ALPHA2 = /^[A-Z]{2}$/;

/** UK is the classic "filter never matches" mistake — the ISO-3166 code is GB. */
export function validateCountryValue(value: string): string | null {
  const tokens = value
    .split(",")
    .map((v) => v.trim().toUpperCase())
    .filter(Boolean);
  if (tokens.includes("UK")) return `"UK" is not an ISO-3166 code — use "GB" for United Kingdom`;
  const invalid = tokens.find((t) => !ISO_ALPHA2.test(t));
  if (invalid) return `"${invalid}" is not a 2-letter ISO-3166 country code (e.g. US, GB, DE)`;
  return null;
}

/**
 * Best-effort client-side heuristic for RE2 compatibility — flags the most
 * common PCRE-only constructs (lookaround, backreferences, atomic groups,
 * possessive quantifiers) that Go's stdlib `regexp` (RE2) rejects. This is a
 * first pass only; real enforcement is compiling with RE2 at save time on
 * the backend (§5 regex safety) — never trust the client as the source of
 * truth here.
 */
export function checkRE2Compatible(pattern: string): string | null {
  if (!pattern) return "Pattern is required";
  if (pattern.length > 200) return "Pattern too long (200 char max)";
  if (/\(\?[=!]/.test(pattern)) return "Lookahead assertions aren't supported by RE2";
  if (/\(\?<[=!]/.test(pattern)) return "Lookbehind assertions aren't supported by RE2";
  if (/\\[1-9]/.test(pattern)) return "Backreferences aren't supported by RE2";
  if (/\(\?>/.test(pattern)) return "Atomic groups aren't supported by RE2";
  if (/[*+?}]\+/.test(pattern)) return "Possessive quantifiers aren't supported by RE2";
  try {
    new RegExp(pattern);
  } catch {
    return "Invalid regular expression syntax";
  }
  return null;
}
