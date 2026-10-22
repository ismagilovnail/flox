"use client";

import * as React from "react";
import { toast } from "sonner";
import { useForm, Controller } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { DownloadIcon, Trash2Icon, UploadIcon } from "lucide-react";
import type { ColumnDef } from "@tanstack/react-table";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Button } from "@/components/ui/button";
import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { DataTable, dataTableFeatures } from "@/components/ui/data-table";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { formatUsd } from "@/lib/format";
import { ApiError } from "@/lib/api/client";
import { countryOptions } from "@/lib/countries";
import { usePermission } from "@/hooks/use-auth";
import { useTrafficSources } from "@/hooks/use-traffic-sources";
import { useCostEntries, useDeleteCostEntry, useImportCostEntries, useUpsertCostEntry } from "@/hooks/use-cost-entries";
import type { CostEntry } from "@/lib/api/cost-entries";

const ALL_SOURCES = "__all__";
const NO_COUNTRY = "__total__";

/** Factory, not a module-level const — see source-form-sheet.tsx's
 * buildSourceFormSchema for why (Zod messages are user-facing text and
 * need the live translator). */
function buildCostEntrySchema(t: TFunction) {
  return z.object({
    entryDate: z.string().min(1, t("form.validation.required", { ns: "cost" })),
    trafficSourceId: z.string(),
    country: z.string(),
    amount: z
      .string()
      .min(1, t("form.validation.required", { ns: "cost" }))
      .refine((v) => !isNaN(Number(v)) && Number(v) >= 0, t("form.validation.amountInvalid", { ns: "cost" })),
    currency: z.string().length(3, t("form.validation.currencyInvalid", { ns: "cost" })),
  });
}

type CostEntryFormValues = z.infer<ReturnType<typeof buildCostEntrySchema>>;

function today(): string {
  return new Date().toISOString().slice(0, 10);
}

function costEntryColumns(
  t: TFunction,
  sourceNameById: Record<string, string>,
  onDeleteRequest: (id: string) => void,
  canWrite: boolean,
): ColumnDef<typeof dataTableFeatures, CostEntry>[] {
  return [
    { accessorKey: "entryDate", header: t("columns.date", { ns: "cost" }) },
    {
      id: "country",
      header: t("columns.country", { ns: "cost" }),
      accessorFn: (row) => row.country || t("columns.noCountry", { ns: "cost" }),
    },
    {
      id: "source",
      header: t("columns.source", { ns: "cost" }),
      // `cell`, not `accessorFn` — see features/campaigns/campaign-
      // columns.tsx's own comment: accessorFn's return value is cached per
      // row and only re-evaluated when the `data` array's reference
      // changes, not when sourceNameById (a separate, later-resolving
      // query) changes.
      cell: ({ row }) =>
        row.original.trafficSourceId
          ? (sourceNameById[row.original.trafficSourceId] ?? row.original.trafficSourceId)
          : t("form.allSources", { ns: "cost" }),
    },
    {
      id: "amount",
      header: t("columns.amount", { ns: "cost" }),
      accessorFn: (row) => `${row.amount.toLocaleString("en-US", { minimumFractionDigits: 2 })} ${row.currency}`,
    },
    {
      id: "amountUsd",
      header: t("columns.amountUsd", { ns: "cost" }),
      cell: ({ row }) =>
        row.original.amountUsd === null ? (
          <span className="text-xs text-muted-foreground">{t("columns.pendingFxRate", { ns: "cost" })}</span>
        ) : (
          formatUsd(row.original.amountUsd, 2)
        ),
    },
    {
      id: "actions",
      header: "",
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) => (
        <div className="flex justify-end">
          {canWrite && (
            <IconButton aria-label={t("list.deleteAria", { ns: "cost" })} onClick={() => onDeleteRequest(row.original.id)}>
              <Trash2Icon className="size-4" />
            </IconButton>
          )}
        </div>
      ),
    },
  ];
}

/** §27-COST's MVP: manual cost entry per campaign/source/day/country, and
 * CSV import. Re-submitting the same (source, date, country) updates that
 * entry in place — the backend upserts, it never stacks
 * (apps/internal/cost). */
/** canEdit false: someone else's campaign (audit TEAM-1) — read-only. */
export function CampaignCostEntries({ campaignId, canEdit = true }: { campaignId: string; canEdit?: boolean }) {
  const { t, i18n } = useTranslation("cost");
  const countries = React.useMemo(() => countryOptions(i18n.language), [i18n.language]);
  const canWrite = usePermission("campaign.write") && canEdit;
  const entriesQuery = useCostEntries(campaignId);
  const sourcesQuery = useTrafficSources();
  const upsert = useUpsertCostEntry(campaignId);
  const del = useDeleteCostEntry(campaignId);

  const [confirmDeleteId, setConfirmDeleteId] = React.useState<string | null>(null);

  const sources = sourcesQuery.data?.trafficSources ?? [];
  const sourceNameById = React.useMemo(() => {
    const map: Record<string, string> = {};
    for (const s of sourcesQuery.data?.trafficSources ?? []) map[s.id] = s.name;
    return map;
  }, [sourcesQuery.data]);

  const form = useForm<CostEntryFormValues>({
    resolver: zodResolver(buildCostEntrySchema(t)),
    defaultValues: { entryDate: today(), trafficSourceId: ALL_SOURCES, country: NO_COUNTRY, amount: "0", currency: "USD" },
  });

  function onSubmit(values: CostEntryFormValues) {
    upsert.mutate(
      {
        entryDate: values.entryDate,
        trafficSourceId: values.trafficSourceId === ALL_SOURCES ? null : values.trafficSourceId,
        country: values.country === NO_COUNTRY ? "" : values.country,
        amount: Number(values.amount),
        currency: values.currency.toUpperCase(),
      },
      {
        onSuccess: () => {
          toast(t("toast.saved"), {
            description: t("toast.savedDescription", {
              date: values.entryDate,
              amount: values.amount,
              currency: values.currency.toUpperCase(),
            }),
          });
          form.reset({ entryDate: today(), trafficSourceId: ALL_SOURCES, country: NO_COUNTRY, amount: "0", currency: "USD" });
        },
        onError: (err) => toast.error(t("toast.saveError"), { description: err.message }),
      },
    );
  }

  function handleDelete(id: string) {
    del.mutate(id, {
      onSuccess: () => {
        setConfirmDeleteId(null);
        toast(t("toast.removed"));
      },
      onError: (err) => {
        setConfirmDeleteId(null);
        toast.error(t("toast.removeError"), { description: err.message });
      },
    });
  }

  const {
    register,
    handleSubmit,
    control,
    formState: { errors, isSubmitting },
  } = form;

  return (
    <div className="flex flex-col gap-6">
      {canWrite && (
        <Card>
          <CardHeader>
            <CardTitle>{t("form.title")}</CardTitle>
            <CardDescription>{t("form.description")}</CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleSubmit(onSubmit)} className="grid max-w-4xl grid-cols-2 gap-4 sm:grid-cols-5 [&>div]:min-w-0">
              <div className="grid gap-1.5">
                <Label htmlFor="entryDate">{t("form.dateLabel")}</Label>
                <Input id="entryDate" type="date" {...register("entryDate")} aria-invalid={!!errors.entryDate} />
                {errors.entryDate && <p className="text-xs text-danger">{errors.entryDate.message}</p>}
              </div>

              <div className="grid gap-1.5">
                <Label htmlFor="trafficSourceId">{t("form.sourceLabel")}</Label>
                <Controller
                  control={control}
                  name="trafficSourceId"
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger id="trafficSourceId" className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value={ALL_SOURCES}>{t("form.allSources")}</SelectItem>
                        {sources.map((s) => (
                          <SelectItem key={s.id} value={s.id}>
                            {s.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  )}
                />
              </div>

              <div className="grid gap-1.5">
                <Label htmlFor="country">{t("form.countryLabel")}</Label>
                <Controller
                  control={control}
                  name="country"
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger id="country" className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value={NO_COUNTRY}>{t("form.allCountries")}</SelectItem>
                        {countries.map((c) => (
                          <SelectItem key={c.value} value={c.value}>
                            {c.value} · {c.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  )}
                />
              </div>

              <div className="grid gap-1.5">
                <Label htmlFor="amount">{t("form.amountLabel")}</Label>
                <Input id="amount" type="number" step="0.01" min="0" {...register("amount")} aria-invalid={!!errors.amount} />
                {errors.amount && <p className="text-xs text-danger">{errors.amount.message}</p>}
              </div>

              <div className="grid gap-1.5">
                <Label htmlFor="currency">{t("form.currencyLabel")}</Label>
                <Input
                  id="currency"
                  placeholder={t("form.currencyPlaceholder")}
                  maxLength={3}
                  {...register("currency")}
                  aria-invalid={!!errors.currency}
                />
                {errors.currency && <p className="text-xs text-danger">{errors.currency.message}</p>}
              </div>

              <div className="col-span-full flex justify-end">
                <Button type="submit" disabled={isSubmitting}>
                  {t("form.submitButton")}
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      )}

      {canWrite && <CostCsvImport campaignId={campaignId} />}

      {entriesQuery.isPending ? (
        <LoadingState label={t("list.loading")} />
      ) : entriesQuery.isError ? (
        <ErrorState title={t("list.loadError")} description={entriesQuery.error.message} onRetry={() => entriesQuery.refetch()} />
      ) : (
        <DataTable
          columns={costEntryColumns(t, sourceNameById, setConfirmDeleteId, canWrite)}
          data={entriesQuery.data.entries}
          emptyTitle={t("list.emptyTitle")}
          emptyDescription={t("list.emptyDescription")}
          pageSize={10}
          exportFilename="cost-entries"
          getRowId={(row) => row.id}
        />
      )}

      <Dialog open={confirmDeleteId !== null} onOpenChange={(open) => !open && setConfirmDeleteId(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("list.deleteConfirmTitle")}</DialogTitle>
            <DialogDescription>{t("list.deleteConfirmDescription")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmDeleteId(null)}>
              {t("actions.cancel", { ns: "common" })}
            </Button>
            <Button
              variant="destructive"
              onClick={() => confirmDeleteId && handleDelete(confirmDeleteId)}
              disabled={del.isPending}
            >
              {t("list.deleteConfirmAction")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

const CSV_TEMPLATE = "date,country,amount,currency,traffic_source_id\n2026-01-31,DE,120.50,USD,\n2026-01-31,FR,80,EUR,\n";

/** Sorts "file" first, then "line N" by N. */
function sortImportErrors(fields: Record<string, string>): [string, string][] {
  const lineNo = (k: string) => (k === "file" ? -1 : Number(k.replace(/^line /, "")) || 0);
  return Object.entries(fields).sort(([a], [b]) => lineNo(a) - lineNo(b));
}

/** CSV import card. The backend parses and validates (csvimport.go); this
 * only reads the file and shows the per-line errors of a refused file. */
function CostCsvImport({ campaignId }: { campaignId: string }) {
  const { t } = useTranslation("cost");
  const importMutation = useImportCostEntries(campaignId);
  const inputRef = React.useRef<HTMLInputElement>(null);
  const [rowErrors, setRowErrors] = React.useState<[string, string][]>([]);

  async function onFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    e.target.value = ""; // the same file can be picked again after a fix
    if (!file) return;
    setRowErrors([]);
    let text: string;
    try {
      text = await file.text();
    } catch {
      toast.error(t("import.readError"));
      return;
    }
    importMutation.mutate(text, {
      onSuccess: (res) => toast(t("import.success"), { description: t("import.successDescription", { count: res.written }) }),
      onError: (err) => {
        if (err instanceof ApiError && err.fields && Object.keys(err.fields).length > 0) {
          setRowErrors(sortImportErrors(err.fields));
        }
        toast.error(t("import.error"), { description: err.message });
      },
    });
  }

  function downloadTemplate() {
    const url = URL.createObjectURL(new Blob([CSV_TEMPLATE], { type: "text/csv" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = "cost-template.csv";
    a.click();
    URL.revokeObjectURL(url);
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("import.title")}</CardTitle>
        <CardDescription>{t("import.description")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <div className="flex flex-wrap gap-2">
          <input ref={inputRef} type="file" accept=".csv,text/csv" className="sr-only" tabIndex={-1} onChange={onFile} aria-hidden />
          <Button type="button" onClick={() => inputRef.current?.click()} disabled={importMutation.isPending}>
            <UploadIcon className="size-4" />
            {importMutation.isPending ? t("import.importing") : t("import.chooseFile")}
          </Button>
          <Button type="button" variant="outline" onClick={downloadTemplate}>
            <DownloadIcon className="size-4" />
            {t("import.template")}
          </Button>
        </div>
        {rowErrors.length > 0 && (
          <div role="alert" className="flex flex-col gap-1 rounded-md border border-danger/40 bg-danger/5 p-2.5 text-xs">
            <span className="font-medium text-danger">{t("import.errorsTitle")}</span>
            <ul className="flex max-h-48 flex-col gap-0.5 overflow-y-auto">
              {rowErrors.map(([where, msg]) => (
                <li key={where}>
                  <span className="font-mono tabular-nums">{where}</span>: {msg}
                </li>
              ))}
            </ul>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
