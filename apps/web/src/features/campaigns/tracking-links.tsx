"use client";

import * as React from "react";
import Link from "next/link";
import { toast } from "sonner";
import { useForm, useWatch, Controller } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { CopyIcon, LinkIcon, PauseIcon, PlayIcon, RefreshCwIcon, Trash2Icon } from "lucide-react";
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
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
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
import { usePermission } from "@/hooks/use-auth";
import {
  useCreateTrackingLink,
  useDeleteTrackingLink,
  useTrackingLinks,
  useUpdateTrackingLinkStatus,
} from "@/hooks/use-tracking-links";
import type { TrackingLink, TrackingLinkStatus } from "@/lib/api/tracking-links";
import { slugify } from "@/lib/utils";
import { TrackingLinkBuilder } from "@/features/campaigns/tracking-link-builder";
import { platformForSourceType, type LinkPlatform } from "@/lib/tracking-link-params";
import { useTrafficSource } from "@/hooks/use-traffic-sources";

/** Appends -2, -3, ... until the slug isn't already used by one of this
 * campaign's existing tracking links (slugs aren't globally unique, but
 * dodging same-campaign collisions saves the operator a round trip to the
 * create-link error toast for the common case: reusing the campaign name). */
function uniqueSlug(base: string, taken: Set<string>) {
  if (!taken.has(base)) return base;
  let i = 2;
  while (taken.has(`${base}-${i}`)) i++;
  return `${base}-${i}`;
}

const RANDOM_SUFFIX_CHARS = "abcdefghijklmnopqrstuvwxyz0123456789";

function randomSuffix(length = 4) {
  let s = "";
  for (let i = 0; i < length; i++) s += RANDOM_SUFFIX_CHARS[Math.floor(Math.random() * RANDOM_SUFFIX_CHARS.length)];
  return s;
}

/** Unlike uniqueSlug (deterministic -2/-3 suffix), this gives a visibly
 * different result on every call — what the "regenerate" button needs, since
 * re-deriving the same campaign-name slug twice looks like a no-op click. */
function randomizedSlug(base: string, taken: Set<string>) {
  let candidate = `${base}-${randomSuffix()}`;
  while (taken.has(candidate)) candidate = `${base}-${randomSuffix()}`;
  return candidate;
}

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
  disabled: boolean,
  platform: LinkPlatform | null,
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
          <TrackingLinkBuilder url={row.original.url} defaultPlatform={platform} />
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
        <div className="flex justify-end gap-1">
          {row.original.status !== "archived" && (
            <Tooltip>
              <TooltipTrigger asChild>
                <IconButton
                  aria-label={row.original.status === "active" ? t("trackingLinks.pause") : t("trackingLinks.resume")}
                  size="icon-sm"
                  disabled={disabled}
                  onClick={() => onToggleStatus(row.original)}
                >
                  {row.original.status === "active" ? (
                    <PauseIcon className="size-3.5" />
                  ) : (
                    <PlayIcon className="size-3.5" />
                  )}
                </IconButton>
              </TooltipTrigger>
              <TooltipContent>
                {row.original.status === "active" ? t("trackingLinks.pause") : t("trackingLinks.resume")}
              </TooltipContent>
            </Tooltip>
          )}
          <Tooltip>
            <TooltipTrigger asChild>
              <IconButton
                aria-label={t("trackingLinks.deleteAria")}
                size="icon-sm"
                disabled={disabled}
                onClick={() => onDeleteRequest(row.original.id)}
              >
                <Trash2Icon className="size-3.5" />
              </IconButton>
            </TooltipTrigger>
            <TooltipContent>{t("trackingLinks.deleteAria")}</TooltipContent>
          </Tooltip>
        </div>
      ),
    },
  ];
}

export function TrackingLinks({
  campaignId,
  campaignName,
  trafficSourceId,
  disabled = false,
}: {
  campaignId: string;
  campaignName: string;
  /** Picks which ad platform's macros the "copy with parameters" menu lists first. */
  trafficSourceId: string;
  /** Archived campaigns don't route traffic, so a new link would resolve
   * to nothing — locked instead of letting the operator create one that
   * silently does nothing. */
  disabled?: boolean;
}) {
  const { t } = useTranslation(["campaigns", "common"]);
  const canWrite = usePermission("campaign.write");
  const domainsQuery = useDomains();
  const sourceQuery = useTrafficSource(trafficSourceId);
  const platform = platformForSourceType(sourceQuery.data?.type);
  const linksQuery = useTrackingLinks(campaignId);
  const create = useCreateTrackingLink(campaignId);
  const updateStatus = useUpdateTrackingLinkStatus(campaignId);
  const del = useDeleteTrackingLink(campaignId);

  const [confirmDeleteId, setConfirmDeleteId] = React.useState<string | null>(null);

  const trackingDomains = React.useMemo(
    () => (domainsQuery.data?.domains ?? []).filter((d) => d.purpose.includes("tracking")),
    [domainsQuery.data],
  );

  const existingSlugs = React.useMemo(
    () => new Set((linksQuery.data?.trackingLinks ?? []).map((l) => l.slug)),
    [linksQuery.data],
  );

  const form = useForm<LinkFormValues>({
    resolver: zodResolver(buildLinkSchema(t)),
    defaultValues: { domainId: "", slug: uniqueSlug(slugify(campaignName), existingSlugs) },
  });
  const {
    register,
    handleSubmit,
    control,
    reset,
    setValue,
    formState: { errors, isSubmitting, dirtyFields },
  } = form;

  const domainId = useWatch({ control, name: "domainId" });
  const slugPreview = useWatch({ control, name: "slug" });
  const selectedDomain = trackingDomains.find((d) => d.id === domainId);

  // Keep the slug in sync with the auto-generated suggestion until the
  // operator actually types into the field themselves.
  React.useEffect(() => {
    if (dirtyFields.slug) return;
    setValue("slug", uniqueSlug(slugify(campaignName), existingSlugs));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [campaignName, existingSlugs]);

  function regenerateSlug() {
    setValue("slug", randomizedSlug(slugify(campaignName), existingSlugs), { shouldDirty: true, shouldValidate: true });
  }

  function onSubmit(values: LinkFormValues) {
    create.mutate(values, {
      onSuccess: () => {
        toast(t("trackingLinks.toast.created"));
        const nextTaken = new Set(existingSlugs).add(values.slug);
        reset({ domainId: values.domainId, slug: uniqueSlug(slugify(campaignName), nextTaken) });
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
          {!canWrite ? (
            <p className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
              {t("permissionDenied.title", { ns: "common" })}
            </p>
          ) : disabled ? (
            <p className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
              {t("trackingLinks.archivedNotice")}
            </p>
          ) : domainsQuery.isPending ? (
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
            <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-3">
              <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
                <div className="grid gap-1.5 sm:w-56">
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

                <div className="grid flex-1 gap-1.5">
                  <Label htmlFor="tl-slug">{t("trackingLinks.slugLabel")}</Label>
                  <div className="flex gap-2">
                    <Input
                      id="tl-slug"
                      placeholder={t("trackingLinks.slugPlaceholder")}
                      className="font-mono text-sm"
                      {...register("slug")}
                      aria-invalid={!!errors.slug}
                    />
                    <IconButton
                      type="button"
                      aria-label={t("trackingLinks.regenerateSlugAria")}
                      variant="outline"
                      onClick={regenerateSlug}
                    >
                      <RefreshCwIcon className="size-4" />
                    </IconButton>
                  </div>
                  {errors.slug && <p className="text-xs text-danger">{errors.slug.message}</p>}
                </div>

                <Button type="submit" disabled={isSubmitting}>
                  {t("trackingLinks.submitButton")}
                </Button>
              </div>

              <p className="text-xs text-muted-foreground">
                {selectedDomain && slugPreview ? (
                  <Mono className="text-xs text-muted-foreground">
                    http://{selectedDomain.domain}/t/{slugPreview.toLowerCase()}
                  </Mono>
                ) : (
                  t("trackingLinks.slugHelp")
                )}
              </p>
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
          columns={columns(t, handleToggleStatus, setConfirmDeleteId, disabled || !canWrite, platform)}
          data={linksQuery.data.trackingLinks}
          emptyTitle={t("trackingLinks.emptyTitle")}
          emptyDescription={t("trackingLinks.emptyDescription")}
          pageSize={10}
          enableSearch={false}
          exportFilename="tracking-links"
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
