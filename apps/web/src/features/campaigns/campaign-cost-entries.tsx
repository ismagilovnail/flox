"use client";

import * as React from "react";
import { toast } from "sonner";
import { useForm, Controller } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Trash2Icon } from "lucide-react";
import type { ColumnDef } from "@tanstack/react-table";

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

const costEntrySchema = z.object({
  entryDate: z.string().min(1, "Required"),
  trafficSourceId: z.string(),
  amount: z
    .string()
    .min(1, "Required")
    .refine((v) => !isNaN(Number(v)) && Number(v) >= 0, "Must be a non-negative number"),
  currency: z.string().length(3, "3-letter code, e.g. USD"),
});

type CostEntryFormValues = z.infer<typeof costEntrySchema>;

function today(): string {
  return new Date().toISOString().slice(0, 10);
}

function costEntryColumns(sourceNameById: Record<string, string>, onDelete: (id: string) => void): ColumnDef<typeof dataTableFeatures, CostEntry>[] {
  return [
    { accessorKey: "entryDate", header: "Date" },
    {
      id: "source",
      header: "Source",
      accessorFn: (row) => (row.trafficSourceId ? (sourceNameById[row.trafficSourceId] ?? row.trafficSourceId) : "All sources"),
    },
    {
      id: "amount",
      header: "Amount",
      accessorFn: (row) => `${row.amount.toLocaleString("en-US", { minimumFractionDigits: 2 })} ${row.currency}`,
    },
    {
      id: "amountUsd",
      header: "USD",
      cell: ({ row }) =>
        row.original.amountUsd === null ? (
          <span className="text-xs text-muted-foreground">pending FX rate</span>
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
          <IconButton aria-label="Delete entry" onClick={() => onDelete(row.original.id)}>
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
    resolver: zodResolver(costEntrySchema),
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
          toast("Spend saved", { description: `${values.entryDate} — ${values.amount} ${values.currency.toUpperCase()}` });
          form.reset({ entryDate: today(), trafficSourceId: ALL_SOURCES, amount: "0", currency: "USD" });
        },
        onError: (err) => toast.error("Couldn't save spend", { description: err.message }),
      },
    );
  }

  function handleDelete(id: string) {
    del.mutate(id, {
      onSuccess: () => toast("Entry removed"),
      onError: (err) => toast.error("Couldn't remove entry", { description: err.message }),
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
          <CardTitle>Add or update spend</CardTitle>
          <CardDescription>
            One entry per day (optionally per source) — entering a day that already has spend updates it, it never stacks.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit(onSubmit)} className="grid max-w-2xl grid-cols-2 gap-4 sm:grid-cols-4">
            <div className="grid gap-1.5">
              <Label htmlFor="entryDate">Date</Label>
              <Input id="entryDate" type="date" {...register("entryDate")} aria-invalid={!!errors.entryDate} />
              {errors.entryDate && <p className="text-xs text-danger">{errors.entryDate.message}</p>}
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="trafficSourceId">Source</Label>
              <Controller
                control={control}
                name="trafficSourceId"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="trafficSourceId" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value={ALL_SOURCES}>All sources</SelectItem>
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
              <Label htmlFor="amount">Amount</Label>
              <Input id="amount" type="number" step="0.01" min="0" {...register("amount")} aria-invalid={!!errors.amount} />
              {errors.amount && <p className="text-xs text-danger">{errors.amount.message}</p>}
            </div>

            <div className="grid gap-1.5">
              <Label htmlFor="currency">Currency</Label>
              <Input id="currency" placeholder="USD" maxLength={3} {...register("currency")} aria-invalid={!!errors.currency} />
              {errors.currency && <p className="text-xs text-danger">{errors.currency.message}</p>}
            </div>

            <div className="col-span-full flex justify-end">
              <Button type="submit" disabled={isSubmitting}>
                Save spend
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      {entriesQuery.isPending ? (
        <LoadingState label="Loading spend entries…" />
      ) : entriesQuery.isError ? (
        <ErrorState title="Couldn't load spend entries" description={entriesQuery.error.message} onRetry={() => entriesQuery.refetch()} />
      ) : (
        <DataTable
          columns={costEntryColumns(sourceNameById, handleDelete)}
          data={entriesQuery.data.entries}
          emptyTitle="No spend logged yet"
          emptyDescription="Add a day's spend above to start tracking Profit/ROI/CPA on the Overview tab."
          pageSize={10}
          getRowId={(row) => row.id}
        />
      )}
    </div>
  );
}
