"use client";

import * as React from "react";
import { ChevronRightIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { cn } from "@/lib/utils";
import { constraints, expandable, refName, resolveRef, typeLabel, type JsonSchema, type OpenApiDocument } from "@/lib/openapi";
import { RichText } from "@/features/api-reference/rich-text";

/** A schema's fields as a nested list: name, type, required, rules,
 * allowed values, description. Objects and references expand on demand;
 * a reference already open above (FilterNode's children) is not opened
 * again. */
export function SchemaView({ doc, schema }: { doc: OpenApiDocument; schema: JsonSchema }) {
  return (
    <div className="rounded-md border border-border">
      <Fields doc={doc} schema={schema} open={new Set()} depth={0} />
    </div>
  );
}

function Fields({ doc, schema, open, depth }: { doc: OpenApiDocument; schema: JsonSchema; open: Set<string>; depth: number }) {
  const { t } = useTranslation("settings");
  let target = schema;
  let ancestors = open;
  if (schema.$ref) {
    target = resolveRef(doc, schema.$ref) ?? {};
    ancestors = new Set(open).add(schema.$ref);
  }
  if (target.anyOf) {
    const named = target.anyOf.find((s) => s.$ref || s.properties);
    if (named) return <Fields doc={doc} schema={named} open={ancestors} depth={depth} />;
  }
  if (target.items && !target.properties) {
    return <Fields doc={doc} schema={target.items} open={ancestors} depth={depth} />;
  }

  const props = Object.entries(target.properties ?? {});
  const extra = typeof target.additionalProperties === "object" ? target.additionalProperties : undefined;
  if (props.length === 0 && !extra) {
    return (
      <div className="px-3 py-2">
        <code className="font-mono text-xs text-muted-foreground">{typeLabel(target)}</code>
        <ValueNotes schema={target} />
      </div>
    );
  }
  const required = new Set(target.required ?? []);
  // Required fields first, each half in name order (the served JSON sorts keys).
  props.sort(([a], [b]) => Number(required.has(b)) - Number(required.has(a)) || a.localeCompare(b));
  return (
    <ul className="divide-y divide-border">
      {props.map(([name, prop]) => (
        <Field key={name} doc={doc} name={name} schema={prop} required={required.has(name)} open={ancestors} depth={depth} />
      ))}
      {extra && <Field doc={doc} name={t("apiReference.anyKey")} schema={extra} required={false} open={ancestors} depth={depth} />}
    </ul>
  );
}

function Field({
  doc,
  name,
  schema,
  required,
  open,
  depth,
}: {
  doc: OpenApiDocument;
  name: string;
  schema: JsonSchema;
  required: boolean;
  open: Set<string>;
  depth: number;
}) {
  const { t } = useTranslation("settings");
  const [expanded, setExpanded] = React.useState(false);
  const resolved = schema.$ref ? (resolveRef(doc, schema.$ref) ?? {}) : schema;
  // A reference to a plain value (an enum like FilterField) shows its values
  // inline; only objects, and arrays of them, expand.
  let child = expandable(schema);
  if (child?.$ref) {
    const target = resolveRef(doc, child.$ref);
    if (!target?.properties && !target?.items && !target?.anyOf) child = undefined;
  }
  const recursive = !!child?.$ref && open.has(child.$ref);
  const canExpand = !!child && !recursive && depth < 8;
  const description = schema.description ?? resolved.description;

  return (
    <li className="px-3 py-2">
      <div className="flex flex-wrap items-baseline gap-x-2 gap-y-1">
        {canExpand ? (
          <button
            type="button"
            onClick={() => setExpanded((v) => !v)}
            aria-expanded={expanded}
            className="inline-flex items-center gap-1 rounded font-mono text-xs font-medium text-foreground hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <ChevronRightIcon className={cn("size-3 transition-transform duration-150", expanded && "rotate-90")} />
            {name}
          </button>
        ) : (
          <span className="pl-4 font-mono text-xs font-medium text-foreground">{name}</span>
        )}
        <code className="font-mono text-xs text-info">{typeLabel(schema)}</code>
        {required && <span className="text-xs text-warning">{t("apiReference.required")}</span>}
        {schema.readOnly && <span className="text-xs text-muted-foreground">{t("apiReference.readOnly")}</span>}
        {recursive && child?.$ref && (
          <span className="text-xs text-muted-foreground">{t("apiReference.recursive", { name: refName(child.$ref) })}</span>
        )}
      </div>
      <div className="pl-4">
        <ValueNotes schema={resolved} />
        <RichText text={description} className="mt-1 text-xs" />
      </div>
      {expanded && child && (
        <div className="mt-2 ml-4 rounded-md border border-border">
          <Fields doc={doc} schema={child} open={open} depth={depth + 1} />
        </div>
      )}
    </li>
  );
}

function ValueNotes({ schema }: { schema: JsonSchema }) {
  const { t } = useTranslation("settings");
  const values = schema.enum ?? schema.items?.enum;
  const rules = constraints(schema);
  if (!values?.length && rules.length === 0) return null;
  return (
    <div className="mt-1 flex flex-wrap items-baseline gap-1 text-xs text-muted-foreground">
      {values && values.length > 1 && (
        <>
          <span>{t("apiReference.oneOf")}</span>
          {values.map((v) => (
            <code key={String(v)} className="rounded bg-muted px-1 font-mono text-foreground">
              {v === "" ? '""' : String(v)}
            </code>
          ))}
        </>
      )}
      {rules.map((r) => (
        <span key={r} className="font-mono">
          {r}
        </span>
      ))}
    </div>
  );
}
