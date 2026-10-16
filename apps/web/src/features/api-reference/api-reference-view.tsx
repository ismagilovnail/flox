"use client";

import * as React from "react";
import Link from "next/link";
import { CheckIcon, CopyIcon, DownloadIcon, SearchIcon, SearchXIcon } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/ui/empty-state";
import { ErrorState } from "@/components/ui/error-state";
import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";
import { LoadingState } from "@/components/ui/loading-state";
import { Caption, H1, H2, H3, Mono } from "@/components/ui/typography";
import { useOpenApiDocument } from "@/hooks/use-openapi-document";
import { openApiDocumentUrl, serverUrl } from "@/lib/api/openapi";
import {
  curlExample,
  filterGroups,
  groupOperations,
  requestSchema,
  successResponse,
  type HttpMethod,
  type OpenApiDocument,
  type OperationEntry,
} from "@/lib/openapi";
import { cn } from "@/lib/utils";
import { RichText } from "@/features/api-reference/rich-text";
import { SchemaView } from "@/features/api-reference/schema-view";

const METHOD_VARIANT: Record<HttpMethod, "success" | "info" | "warning" | "danger" | "secondary"> = {
  GET: "success",
  POST: "info",
  PUT: "warning",
  PATCH: "warning",
  DELETE: "danger",
};

function MethodBadge({ method, className }: { method: HttpMethod; className?: string }) {
  return (
    <Badge variant={METHOD_VARIANT[method]} className={cn("w-14 justify-center font-mono text-[10px] tabular-nums", className)}>
      {method}
    </Badge>
  );
}

/** The public API reference (API-1), rendered from the live
 * GET /v1/openapi.json — the same document integrations download, so the
 * page cannot drift from the API. */
export function ApiReferenceView() {
  const { t } = useTranslation("settings");
  const query = useOpenApiDocument();

  return (
    <div className="mx-auto flex w-full max-w-7xl flex-col gap-6 px-4 py-6 sm:px-6">
      <header className="flex flex-wrap items-end justify-between gap-4 border-b border-border pb-4">
        <div className="flex flex-col gap-1">
          <Caption className="uppercase tracking-widest text-muted-foreground">FLOX</Caption>
          <H1>{t("apiReference.title")}</H1>
          {query.data && (
            <Caption className="text-muted-foreground">
              {t("apiReference.version", { version: query.data.info.version })} ·{" "}
              <Mono className="text-xs">{serverUrl(query.data)}</Mono>
            </Caption>
          )}
        </div>
        <div className="flex flex-wrap gap-2">
          <Button asChild variant="outline" size="sm">
            <a href={openApiDocumentUrl("json")} download="flox-openapi.json">
              <DownloadIcon className="size-4" /> openapi.json
            </a>
          </Button>
          <Button asChild variant="outline" size="sm">
            <a href={openApiDocumentUrl("yaml")} download="flox-openapi.yaml">
              <DownloadIcon className="size-4" /> openapi.yaml
            </a>
          </Button>
          <Button asChild variant="ghost" size="sm">
            <Link href="/settings?tab=api-keys">{t("apiReference.manageKeys")}</Link>
          </Button>
        </div>
      </header>

      {query.isPending && <LoadingState label={t("apiReference.loading")} />}
      {query.isError && (
        <ErrorState title={t("apiReference.loadError")} description={query.error.message} onRetry={() => query.refetch()} />
      )}
      {query.isSuccess && <Reference doc={query.data} />}
    </div>
  );
}

function Reference({ doc }: { doc: OpenApiDocument }) {
  const { t } = useTranslation("settings");
  const [search, setSearch] = React.useState("");
  const groups = React.useMemo(() => groupOperations(doc), [doc]);
  const visible = React.useMemo(() => filterGroups(groups, search), [groups, search]);
  const server = serverUrl(doc);

  return (
    <div className="grid gap-8 lg:grid-cols-[16rem_minmax(0,1fr)]">
      <nav aria-label={t("apiReference.navLabel")} className="flex flex-col gap-4 lg:sticky lg:top-6 lg:max-h-[calc(100dvh-3rem)] lg:self-start lg:overflow-y-auto">
        <div className="relative">
          <SearchIcon className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t("apiReference.search")}
            aria-label={t("apiReference.search")}
            className="pl-8"
          />
        </div>
        <ul className="hidden flex-col gap-4 lg:flex">
          {visible.map((g) => (
            <li key={g.name} className="flex flex-col gap-1">
              <a href={`#tag-${slug(g.name)}`} className="text-xs font-medium uppercase tracking-wide text-muted-foreground hover:text-foreground">
                {g.name}
              </a>
              <ul className="flex flex-col">
                {g.operations.map((e) => (
                  <li key={e.operation.operationId}>
                    <a
                      href={`#${e.operation.operationId}`}
                      className="flex items-center gap-2 rounded px-1 py-0.5 text-xs text-muted-foreground hover:bg-muted hover:text-foreground"
                    >
                      <span className={cn("w-11 shrink-0 font-mono text-[10px]", methodText(e.method))}>{e.method}</span>
                      <span className="truncate">{e.operation.summary}</span>
                    </a>
                  </li>
                ))}
              </ul>
            </li>
          ))}
        </ul>
      </nav>

      <main className="flex min-w-0 flex-col gap-10">
        {!search && (
          <section className="flex max-w-3xl flex-col gap-3">
            <H2>{doc.info.title}</H2>
            <RichText text={doc.info.description} />
          </section>
        )}
        {visible.length === 0 && (
          <EmptyState icon={SearchXIcon} title={t("apiReference.noMatch")} description={t("apiReference.noMatchHint")} />
        )}
        {visible.map((g) => (
          <section key={g.name} id={`tag-${slug(g.name)}`} className="flex scroll-mt-6 flex-col gap-6">
            <div className="flex flex-col gap-1 border-b border-border pb-2">
              <H2>{g.name}</H2>
              <RichText text={g.description} />
            </div>
            {g.operations.map((e) => (
              <OperationCard key={e.operation.operationId} doc={doc} server={server} entry={e} />
            ))}
          </section>
        ))}
      </main>
    </div>
  );
}

function OperationCard({ doc, server, entry }: { doc: OpenApiDocument; server: string; entry: OperationEntry }) {
  const { t } = useTranslation("settings");
  const { operation, method, path, parameters } = entry;
  const body = requestSchema(entry);
  const success = successResponse(entry);
  const curl = curlExample(doc, server, entry);

  return (
    <article id={operation.operationId} className="flex scroll-mt-6 flex-col gap-4 rounded-lg border border-border bg-card p-4">
      <div className="flex flex-col gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <MethodBadge method={method} />
          <Mono className="min-w-0 break-all text-sm text-foreground">{path}</Mono>
          <Badge variant={operation["x-flox-scope"] === "write" ? "secondary" : "outline"} className="ml-auto">
            {t("apiReference.scope", { scope: t(`apiKeys.scopes.${operation["x-flox-scope"]}`) })}
          </Badge>
        </div>
        <H3>{operation.summary}</H3>
        <RichText text={operation.description} />
      </div>

      {parameters.length > 0 && (
        <div className="flex flex-col gap-2">
          <Caption className="font-medium uppercase tracking-wide text-muted-foreground">{t("apiReference.parameters")}</Caption>
          <ul className="divide-y divide-border rounded-md border border-border">
            {parameters.map((p) => (
              <li key={`${p.in}-${p.name}`} className="flex flex-wrap items-baseline gap-x-2 gap-y-1 px-3 py-2">
                <span className="font-mono text-xs font-medium">{p.name}</span>
                <span className="text-xs text-muted-foreground">{t(`apiReference.in.${p.in}`)}</span>
                {p.schema && <code className="font-mono text-xs text-info">{schemaHint(p.schema)}</code>}
                {p.required && <span className="text-xs text-warning">{t("apiReference.required")}</span>}
                {p.description && <span className="basis-full text-xs text-muted-foreground">{p.description}</span>}
              </li>
            ))}
          </ul>
        </div>
      )}

      {body && (
        <div className="flex flex-col gap-2">
          <Caption className="font-medium uppercase tracking-wide text-muted-foreground">{t("apiReference.body")}</Caption>
          <SchemaView doc={doc} schema={body} />
        </div>
      )}

      {success && (
        <div className="flex flex-col gap-2">
          <Caption className="font-medium uppercase tracking-wide text-muted-foreground">
            {t("apiReference.response", { status: success.status })}
            {success.description ? ` — ${success.description}` : ""}
          </Caption>
          {success.schema ? <SchemaView doc={doc} schema={success.schema} /> : <Caption>{t("apiReference.noBody")}</Caption>}
        </div>
      )}

      <CodeBlock code={curl} />
    </article>
  );
}

function CodeBlock({ code }: { code: string }) {
  const { t } = useTranslation("settings");
  const [copied, setCopied] = React.useState(false);
  async function copy() {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      toast.error(t("apiReference.copyError"));
    }
  }
  return (
    <div className="relative rounded-md border border-border bg-muted">
      <pre className="overflow-x-auto p-3 pr-10 font-mono text-xs leading-relaxed text-foreground">{code}</pre>
      <IconButton aria-label={t("apiReference.copyCurl")} size="icon-sm" onClick={copy} className="absolute top-2 right-2">
        {copied ? <CheckIcon className="size-3.5 text-success" /> : <CopyIcon className="size-3.5" />}
      </IconButton>
    </div>
  );
}

function schemaHint(schema: { type?: string | string[]; format?: string; enum?: unknown[]; $ref?: string; default?: unknown; maximum?: number }): string {
  const type = Array.isArray(schema.type) ? schema.type.join(" | ") : (schema.type ?? (schema.$ref ? schema.$ref.split("/").pop() : ""));
  const parts = [schema.format ? `${type} · ${schema.format}` : type];
  if (schema.maximum !== undefined) parts.push(`≤ ${schema.maximum}`);
  if (schema.default !== undefined) parts.push(`default ${JSON.stringify(schema.default)}`);
  return parts.join(", ");
}

function methodText(method: HttpMethod): string {
  return { GET: "text-success", POST: "text-info", PUT: "text-warning", PATCH: "text-warning", DELETE: "text-danger" }[method];
}

function slug(s: string): string {
  return s.toLowerCase().replace(/[^a-z0-9]+/g, "-");
}
