"use client";

import * as React from "react";
import Link from "next/link";
import { toast } from "sonner";
import { useForm, useWatch, Controller } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { CopyIcon, LinkIcon, MoreHorizontalIcon, PauseIcon, PlayIcon, Trash2Icon } from "lucide-react";
import type { ColumnDef } from "@tanstack/react-table";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Mono } from "@/components/ui/typography";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
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
import { EmptyState } from "@/components/ui/empty-state";
import { useDomains } from "@/hooks/use-domains";
import {
  useCreateTrackingLink,
  useDeleteTrackingLink,
  useTrackingLinks,
  useUpdateTrackingLinkStatus,
} from "@/hooks/use-tracking-links";
import type { TrackingLink, TrackingLinkStatus } from "@/lib/api/tracking-links";

const STATUS_VARIANT: Record<TrackingLinkStatus, "success" | "warning" | "secondary"> = {
  active: "success",
  paused: "warning",
  archived: "secondary",
};

function buildLinkSchema(t: TFunction) {
  return z.object({
    domainId: z.string().min(1, t("trackingLinks.domainLabel") + " *"),
    slug: z
      .string()
      .toLowerCase()
      .regex(/^[a-z0-9]+(-[a-z0-9]+)*$/, t("trackingLinks.slugHelp")),
  });
}

type LinkFormValues = z.infer<ReturnType<typeof buildLinkSchema>>;

function columns(
  t: TFunction,
  onToggleStatus: (link: TrackingLink) => void,
  onDeleteRequest: (id: string) => void,
): ColumnDef<typeof dataTableFeatures, TrackingLink>[] {
  return [
    {
      id: "url",
      header: t("trackingLinks.urlHeader"),
      accessorFn: (row) => row.url,
      cell: ({ row }) => (
        <div className="flex items-center gap-1.5">
          <Mono className="text-sm">{row.original.url}</Mono>
          <IconButton
            aria-label={t("trackingLinks.copyAria")}
            size="icon-sm"
            variant="ghost"
            onClick={() => {
              navigator.clipboard.writeText(row.original.url);
              toast(t("trackingLinks.copiedToast"));
            }}
          >
            <CopyIcon className="size-3.5" />
          </IconButton>
        </div>
      ),
    },
    {
      accessorKey: "status",
      header: t("trackingLinks.statusHeader"),
      cell: ({ getValue }) => {
        const status = getValue() as TrackingLinkStatus;
        return <Badge variant={STATUS_VARIANT[status]}>{t(`status.${status}`, { ns: "common" })}</Badge>;
      },
    },
    {
      id: "actions",
      header: "",
      enableSorting: false,
      enableHiding: false,
      cell: ({ row }) => (
        <div className="flex justify-end">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <IconButton aria-label={t("rowActions.actionsAria", { name: row.original.slug })} variant="ghost" size="icon-sm">
                <MoreHorizontalIcon className="size-4" />
              </IconButton>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {row.original.status !== "archived" && (
                <DropdownMenuItem onSelect={() => onToggleStatus(row.original)}>
                  {row.original.status === "active" ? (
                    <>
                      <PauseIcon className="size-4" /> {t("trackingLinks.pause")}
                    </>
                  ) : (
                    <>
                      <PlayIcon className="size-4" /> {t("trackingLinks.resume")}
                    </>
                  )}
                </DropdownMenuItem>
              )}
              <DropdownMenuItem variant="destructive" onSelect={() => onDeleteRequest(row.original.id)}>
                <Trash2Icon className="size-4" /> {t("trackingLinks.deleteAria")}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      ),
    },
  ];
}

export function TrackingLinks({ campaignId }: { campaignId: string }) {
  const { t } = useTranslation(["campaigns", "common"]);
  const domainsQuery = useDomains();
  const linksQuery = useTrackingLinks(campaignId);
  const create = useCreateTrackingLink(campaignId);
  const updateStatus = useUpdateTrackingLinkStatus(campaignId);
  const del = useDeleteTrackingLink(campaignId);

  const [confirmDeleteId, setConfirmDeleteId] = React.useState<string | null>(null);

  const trackingDomains = React.useMemo(
    () => (domainsQuery.data?.domains ?? []).filter((d) => d.purpose.includes("tracking")),
    [domainsQuery.data],
  );

  const form = useForm<LinkFormValues>({
    resolver: zodResolver(buildLinkSchema(t)),
    defaultValues: { domainId: "", slug: "" },
  });
  const {
    register,
    handleSubmit,
    control,
    reset,
    formState: { errors, isSubmitting },
  } = form;

  const domainId = useWatch({ control, name: "domainId" });
  const slugPreview = useWatch({ control, name: "slug" });
  const selectedDomain = trackingDomains.find((d) => d.id === domainId);

  function onSubmit(values: LinkFormValues) {
    create.mutate(values, {
      onSuccess: () => {
        toast(t("trackingLinks.toast.created"));
        reset({ domainId: values.domainId, slug: "" });
      },
      onError: (err) => toast.error(t("trackingLinks.toast.createError"), { description: err.message }),
    });
  }

  function handleToggleStatus(link: TrackingLink) {
    const next: TrackingLinkStatus = link.status === "active" ? "paused" : "active";
    updateStatus.mutate(
      { id: link.id, status: next },
      {
        onSuccess: () => toast(t("trackingLinks.toast.updated")),
        onError: (err) => toast.error(t("trackingLinks.toast.updateError"), { description: err.message }),
      },
    );
  }

  function handleDelete(id: string) {
    del.mutate(id, {
      onSuccess: () => {
        setConfirmDeleteId(null);
        toast(t("trackingLinks.toast.removed"));
      },
      onError: (err) => {
        setConfirmDeleteId(null);
        toast.error(t("trackingLinks.toast.removeError"), { description: err.message });
      },
    });
  }

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{t("trackingLinks.title")}</CardTitle>
          <CardDescription>{t("trackingLinks.description")}</CardDescription>
        </CardHeader>
        <CardContent>
          {domainsQuery.isPending ? (
            <LoadingState label={t("trackingLinks.loading")} />
          ) : trackingDomains.length === 0 ? (
            <EmptyState
              icon={LinkIcon}
              title={t("trackingLinks.noDomainsTitle")}
              description={t("trackingLinks.noDomainsDescription")}
              action={
                <Button asChild size="sm">
                  <Link href="/domains">{t("trackingLinks.manageDomainsLink")}</Link>
                </Button>
              }
            />
          ) : (
            <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4">
              <div className="grid grid-cols-1 items-start gap-4 sm:grid-cols-2">
                <div className="grid gap-1.5">
                  <Label htmlFor="tl-domain">{t("trackingLinks.domainLabel")}</Label>
                  <Controller
                    control={control}
                    name="domainId"
                    render={({ field }) => (
                      <Select value={field.value} onValueChange={field.onChange}>
                        <SelectTrigger id="tl-domain" className="w-full">
                          <SelectValue placeholder={t("trackingLinks.domainPlaceholder")} />
                        </SelectTrigger>
                        <SelectContent>
                          {trackingDomains.map((d) => (
                            <SelectItem key={d.id} value={d.id}>
                              {d.domain}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    )}
                  />
                  {errors.domainId && <p className="text-xs text-danger">{errors.domainId.message}</p>}
                </div>

                <div className="grid gap-1.5">
                  <Label htmlFor="tl-slug">{t("trackingLinks.slugLabel")}</Label>
                  <Input
                    id="tl-slug"
                    placeholder={t("trackingLinks.slugPlaceholder")}
                    className="font-mono text-sm"
                    {...register("slug")}
                    aria-invalid={!!errors.slug}
                  />
                  {errors.slug ? (
                    <p className="text-xs text-danger">{errors.slug.message}</p>
                  ) : (
                    <p className="text-xs text-muted-foreground">{t("trackingLinks.slugHelp")}</p>
                  )}
                </div>
              </div>

              {selectedDomain && slugPreview && (
                <Mono className="text-xs text-muted-foreground">
                  http://{selectedDomain.domain}/t/{slugPreview.toLowerCase()}
                </Mono>
              )}

              <div className="flex justify-end">
                <Button type="submit" disabled={isSubmitting}>
                  {t("trackingLinks.submitButton")}
                </Button>
              </div>
            </form>
          )}
        </CardContent>
      </Card>

      {linksQuery.isPending ? (
        <LoadingState label={t("trackingLinks.loading")} />
      ) : linksQuery.isError ? (
        <ErrorState
          title={t("trackingLinks.loadError")}
          description={linksQuery.error.message}
          onRetry={() => linksQuery.refetch()}
        />
      ) : (
        <DataTable
          columns={columns(t, handleToggleStatus, setConfirmDeleteId)}
          data={linksQuery.data.trackingLinks}
          emptyTitle={t("trackingLinks.emptyTitle")}
          emptyDescription={t("trackingLinks.emptyDescription")}
          pageSize={10}
          getRowId={(row) => row.id}
        />
      )}

      <Dialog open={confirmDeleteId !== null} onOpenChange={(open) => !open && setConfirmDeleteId(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("trackingLinks.deleteConfirmTitle")}</DialogTitle>
            <DialogDescription>{t("trackingLinks.deleteConfirmDescription")}</DialogDescription>
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
              {t("trackingLinks.deleteConfirmAction")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
