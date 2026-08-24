"use client";

import * as React from "react";
import { toast } from "sonner";
import { useForm, Controller } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Trash2Icon } from "lucide-react";
import type { ColumnDef } from "@tanstack/react-table";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Button } from "@/components/ui/button";
import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { DataTable, dataTableFeatures } from "@/components/ui/data-table";
import { LoadingState } from "@/components/ui/loading-state";
import { ErrorState } from "@/components/ui/error-state";
import { formatUsd } from "@/lib/format";
import { useTrafficSources } from "@/hooks/use-traffic-sources";
import { useCostEntries, useDeleteCostEntry, useUpsertCostEntry } from "@/hooks/use-cost-entries";
import type { CostEntry } from "@/lib/api/cost-entries";

const ALL_SOURCES = "__all__";

/** Factory, not a module-level const — see source-form-sheet.tsx's
 * buildSourceFormSchema for why (Zod messages are user-facing text and
 * need the live translator). */
function buildCostEntrySchema(t: TFunction) {
  return z.object({
    entryDate: z.string().min(1, t("form.validation.required", { ns: "cost" })),
    trafficSourceId: z.string(),
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
  onDelete: (id: string) => void,
): ColumnDef<typeof dataTableFeatures, CostEntry>[] {
  return [
    { accessorKey: "entryDate", header: t("columns.date", { ns: "cost" }) },
    {
      id: "source",
      header: t("columns.source", { ns: "cost" }),
      accessorFn: (row) =>
        row.trafficSourceId
          ? (sourceNameById[row.trafficSourceId] ?? row.trafficSourceId)
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
          <IconButton aria-label={t("list.deleteAria", { ns: "cost" })} onClick={() => onDelete(row.original.id)}>
            <Trash2Icon className="size-4" />
          </IconButton>
        </div>
      ),
    },
  ];
}

/** §27-COST's MVP: manual cost entry per campaign/source/day. Re-submitting
 * the same (source, date) via the form updates that day in place — the
 * backend upserts, it never stacks (apps/internal/cost). */
export function CampaignCostEntries({ campaignId }: { campaignId: string }) {
  const { t } = useTranslation("cost");
  const entriesQuery = useCostEntries(campaignId);
  const sourcesQuery = useTrafficSources();
  const upsert = useUpsertCostEntry(campaignId);
  const del = useDeleteCostEntry(campaignId);

  const sources = sourcesQuery.data?.trafficSources ?? [];
  const sourceNameById = React.useMemo(() => {
    const map: Record<string, string> = {};
    for (const s of sourcesQuery.data?.trafficSources ?? []) map[s.id] = s.name;
    return map;
  }, [sourcesQuery.data]);

  const form = useForm<CostEntryFormValues>({
    resolver: zodResolver(buildCostEntrySchema(t)),
    defaultValues: { entryDate: today(), trafficSourceId: ALL_SOURCES, amount: "0", currency: "USD" },
  });

  function onSubmit(values: CostEntryFormValues) {
    upsert.mutate(
      {
        entryDate: values.entryDate,
        trafficSourceId: values.trafficSourceId === ALL_SOURCES ? null : values.trafficSourceId,
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
          form.reset({ entryDate: today(), trafficSourceId: ALL_SOURCES, amount: "0", currency: "USD" });
        },
        onError: (err) => toast.error(t("toast.saveError"), { description: err.message }),
      },
    );
  }

  function handleDelete(id: string) {
    del.mutate(id, {
      onSuccess: () => toast(t("toast.removed")),
      onError: (err) => toast.error(t("toast.removeError"), { description: err.message }),
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
      <Card>
        <CardHeader>
          <CardTitle>{t("form.title")}</CardTitle>
          <CardDescription>{t("form.description")}</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit(onSubmit)} className="grid max-w-2xl grid-cols-2 gap-4 sm:grid-cols-4">
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

      {entriesQuery.isPending ? (
        <LoadingState label={t("list.loading")} />
      ) : entriesQuery.isError ? (
        <ErrorState title={t("list.loadError")} description={entriesQuery.error.message} onRetry={() => entriesQuery.refetch()} />
      ) : (
        <DataTable
          columns={costEntryColumns(t, sourceNameById, handleDelete)}
          data={entriesQuery.data.entries}
          emptyTitle={t("list.emptyTitle")}
          emptyDescription={t("list.emptyDescription")}
          pageSize={10}
          getRowId={(row) => row.id}
        />
      )}
    </div>
  );
}
