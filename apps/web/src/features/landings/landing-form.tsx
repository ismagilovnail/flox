"use client";

import * as React from "react";
import Link from "next/link";
import { Controller, useForm, useWatch } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { CodeIcon, FileArchiveIcon, ImagesIcon, UploadCloudIcon, XIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { IconButton } from "@/components/ui/icon-button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Mono } from "@/components/ui/typography";
import { cn } from "@/lib/utils";
import { GalleryFilePickerDialog } from "@/features/content-gallery/gallery-file-picker-dialog";
import { LandingTemplateEditor } from "@/features/landings/landing-template-editor";
import { formatBytes } from "@/features/landings/format-bytes";
import type { Landing, LandingStatus, LandingType } from "@/lib/api/landings";
import { nameField } from "@/schemas/fields";

/** Mirrors apps/internal/landing/bundle.go's maxArchiveBytes — checked
 * here only to fail fast before uploading; the server enforces it. */
const MAX_ARCHIVE_BYTES = 50 * 1024 * 1024;

/** Factory, not a module-level const — Zod's validation messages are
 * user-facing text and need the live translator, which only exists inside
 * a component. `requireArchive` is true on create: an internal landing
 * can't exist without one. */
export function buildLandingFormSchema(t: TFunction, requireArchive: boolean) {
  return z
    .object({
      name: nameField(t("form.validation.nameMin", { ns: "landings" }), { max: 100, trim: true }),
      title: z.string().trim().max(200),
      logoUrl: z.string(),
      type: z.enum(["internal", "external"] as [LandingType, ...LandingType[]]),
      url: z.string().optional(),
      archive: z.custom<File | null>().nullable(),
      status: z.enum(["active", "paused", "archived"] as [LandingStatus, ...LandingStatus[]]),
    })
    .superRefine((data, ctx) => {
      if (data.type === "external") {
        if (!data.url || !z.url().safeParse(data.url).success) {
          ctx.addIssue({ code: "custom", path: ["url"], message: t("form.validation.urlInvalid", { ns: "landings" }) });
        }
        return;
      }
      if (!data.title) {
        ctx.addIssue({ code: "custom", path: ["title"], message: t("form.validation.titleRequired", { ns: "landings" }) });
      }
      if (!data.logoUrl) {
        ctx.addIssue({ code: "custom", path: ["logoUrl"], message: t("form.validation.logoRequired", { ns: "landings" }) });
      }
      if (requireArchive && !data.archive) {
        ctx.addIssue({ code: "custom", path: ["archive"], message: t("form.validation.archiveRequired", { ns: "landings" }) });
      }
      if (data.archive) {
        if (!data.archive.name.toLowerCase().endsWith(".zip")) {
          ctx.addIssue({ code: "custom", path: ["archive"], message: t("form.validation.archiveNotZip", { ns: "landings" }) });
        } else if (data.archive.size > MAX_ARCHIVE_BYTES) {
          ctx.addIssue({ code: "custom", path: ["archive"], message: t("form.validation.archiveTooLarge", { ns: "landings" }) });
        }
      }
    });
}

export type LandingFormValues = z.infer<ReturnType<typeof buildLandingFormSchema>>;

const STATUS_OPTIONS: LandingStatus[] = ["active", "paused", "archived"];

/** The landing create/edit form. Internal landings are uploaded as a ZIP
 * (index.html / index.twig at the root plus assets); external ones point
 * at a URL the advertiser hosts. `landing` is set when editing — it
 * locks the type, makes the archive optional (re-upload keeps the id),
 * and enables the template editor. */
export function LandingForm({
  title,
  landing,
  defaultValues,
  submitLabel,
  onSubmit,
  disabled = false,
  submitting = false,
}: {
  title: string;
  landing?: Landing;
  defaultValues: Partial<LandingFormValues>;
  submitLabel: string;
  onSubmit: (values: LandingFormValues) => void;
  disabled?: boolean;
  submitting?: boolean;
}) {
  const { t } = useTranslation(["landings", "common"]);
  const isEdit = !!landing;
  const form = useForm<LandingFormValues>({
    resolver: zodResolver(buildLandingFormSchema(t, !isEdit)),
    defaultValues: {
      name: "",
      title: "",
      logoUrl: "",
      type: "internal",
      url: "",
      archive: null,
      status: "active",
      ...defaultValues,
    },
  });

  const {
    register,
    handleSubmit,
    control,
    setValue,
    formState: { errors },
  } = form;

  const type = useWatch({ control, name: "type" });
  const logoUrl = useWatch({ control, name: "logoUrl" });
  const [pickingLogo, setPickingLogo] = React.useState(false);
  const [editingTemplate, setEditingTemplate] = React.useState(false);

  // A successful re-upload resets the picked archive (the parent passes
  // fresh defaultValues for the updated landing).
  const updatedAt = landing?.updatedAt;
  React.useEffect(() => {
    if (updatedAt) setValue("archive", null);
  }, [updatedAt, setValue]);

  const statusOptions = isEdit ? STATUS_OPTIONS : STATUS_OPTIONS.filter((s) => s !== "archived");

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{title}</CardTitle>
          <CardDescription>{t("form.description", { ns: "landings" })}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="flex flex-col gap-4 sm:flex-row">
            <div className="grid gap-1.5">
              <Label id="lnd-logo-label">
                {t("form.logoLabel", { ns: "landings" })}
                {type === "external" && (
                  <span className="text-muted-foreground"> {t("form.optional", { ns: "landings" })}</span>
                )}
              </Label>
              <div className="relative w-fit">
                <button
                  type="button"
                  aria-labelledby="lnd-logo-label"
                  disabled={disabled}
                  onClick={() => setPickingLogo(true)}
                  className={cn(
                    "group relative flex size-24 items-center justify-center overflow-hidden rounded-lg border-2 border-dashed text-muted-foreground transition-colors",
                    errors.logoUrl ? "border-danger" : "border-border hover:border-muted-foreground/50 hover:text-foreground",
                  )}
                >
                  {logoUrl ? (
                    <>
                      {/* eslint-disable-next-line @next/next/no-img-element -- a Content Gallery public object-storage URL, not a Next-optimizable static asset */}
                      <img src={logoUrl} alt="" className="size-full object-cover" />
                      <div className="absolute inset-0 flex items-center justify-center bg-black/50 opacity-0 transition-opacity group-hover:opacity-100">
                        <span className="text-xs font-medium text-white">{t("form.logoChange", { ns: "landings" })}</span>
                      </div>
                    </>
                  ) : (
                    <span className="flex flex-col items-center gap-1 px-2 text-center">
                      <ImagesIcon className="size-5" />
                      <span className="text-[11px] font-medium">{t("form.logoChoose", { ns: "landings" })}</span>
                    </span>
                  )}
                </button>
                {logoUrl && !disabled && (
                  <IconButton
                    type="button"
                    size="icon-xs"
                    variant="secondary"
                    className="absolute -top-1.5 -right-1.5 rounded-full shadow"
                    aria-label={t("form.logoRemoveAria", { ns: "landings" })}
                    onClick={() => setValue("logoUrl", "", { shouldValidate: true, shouldDirty: true })}
                  >
                    <XIcon className="size-3" />
                  </IconButton>
                )}
              </div>
              <p className="text-[11px] text-muted-foreground">{t("form.logoHint", { ns: "landings" })}</p>
              {errors.logoUrl && <p className="text-xs text-danger">{errors.logoUrl.message}</p>}
              <GalleryFilePickerDialog
                open={pickingLogo}
                onOpenChange={setPickingLogo}
                requiredAspect={1}
                onPick={(file) => setValue("logoUrl", file.url, { shouldValidate: true, shouldDirty: true })}
              />
            </div>

            <div className="grid flex-1 content-start gap-4 sm:grid-cols-2">
              <div className="grid gap-1.5">
                <Label htmlFor="lnd-name">{t("form.nameLabel", { ns: "landings" })}</Label>
                <Input
                  id="lnd-name"
                  placeholder={t("form.namePlaceholder", { ns: "landings" })}
                  {...register("name")}
                  aria-invalid={!!errors.name}
                  disabled={disabled}
                />
                {errors.name && <p className="text-xs text-danger">{errors.name.message}</p>}
              </div>

              <div className="grid gap-1.5">
                <Label htmlFor="lnd-title">
                  {t("form.titleLabel", { ns: "landings" })}
                  {type === "external" && (
                    <span className="text-muted-foreground"> {t("form.optional", { ns: "landings" })}</span>
                  )}
                </Label>
                <Input
                  id="lnd-title"
                  placeholder={t("form.titlePlaceholder", { ns: "landings" })}
                  {...register("title")}
                  aria-invalid={!!errors.title}
                  disabled={disabled}
                />
                {errors.title && <p className="text-xs text-danger">{errors.title.message}</p>}
              </div>

              <div className="grid gap-1.5">
                <Label htmlFor="lnd-type">{t("form.typeLabel", { ns: "landings" })}</Label>
                <Controller
                  control={control}
                  name="type"
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange} disabled={disabled || isEdit}>
                      <SelectTrigger id="lnd-type" className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="internal">{t("type.internal", { ns: "landings" })}</SelectItem>
                        <SelectItem value="external">{t("type.external", { ns: "landings" })}</SelectItem>
                      </SelectContent>
                    </Select>
                  )}
                />
                {isEdit && <p className="text-[11px] text-muted-foreground">{t("form.typeLocked", { ns: "landings" })}</p>}
              </div>

              <div className="grid gap-1.5">
                <Label htmlFor="lnd-status">{t("form.statusLabel", { ns: "landings" })}</Label>
                <Controller
                  control={control}
                  name="status"
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange} disabled={disabled}>
                      <SelectTrigger id="lnd-status" className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {statusOptions.map((s) => (
                          <SelectItem key={s} value={s}>
                            {t(`status.${s}`, { ns: "common" })}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  )}
                />
              </div>
            </div>
          </div>

          {type === "external" ? (
            <div className="grid gap-1.5">
              <Label htmlFor="lnd-url">{t("form.urlLabel", { ns: "landings" })}</Label>
              <Input
                id="lnd-url"
                placeholder={t("form.urlPlaceholder", { ns: "landings" })}
                className="font-mono text-xs"
                {...register("url")}
                aria-invalid={!!errors.url}
                disabled={disabled}
              />
              {errors.url && <p className="text-xs text-danger">{errors.url.message}</p>}
            </div>
          ) : (
            <div className="flex flex-col gap-3">
              {landing && <CurrentBundle landing={landing} onEditTemplate={() => setEditingTemplate(true)} disabled={disabled} />}

              <Controller
                control={control}
                name="archive"
                render={({ field }) => (
                  <ArchiveDropzone
                    file={field.value}
                    onChange={(file) => {
                      field.onChange(file);
                      void form.trigger("archive");
                    }}
                    label={isEdit ? t("form.archiveReplaceLabel", { ns: "landings" }) : t("form.archiveLabel", { ns: "landings" })}
                    invalid={!!errors.archive}
                    disabled={disabled}
                  />
                )}
              />
              {isEdit && <p className="text-[11px] text-muted-foreground">{t("form.archiveReplaceHint", { ns: "landings" })}</p>}
              {errors.archive && <p className="text-xs text-danger">{errors.archive.message}</p>}

              <ArchiveRequirements />
            </div>
          )}
        </CardContent>
      </Card>

      <div className="flex justify-end gap-2">
        <Button type="button" variant="outline" asChild>
          <Link href="/landings">{t("actions.cancel", { ns: "common" })}</Link>
        </Button>
        <Button type="submit" disabled={disabled || submitting}>
          {submitLabel}
        </Button>
      </div>

      {landing && landing.type === "internal" && (
        <LandingTemplateEditor landing={landing} open={editingTemplate} onOpenChange={setEditingTemplate} />
      )}
    </form>
  );
}

function CurrentBundle({
  landing,
  onEditTemplate,
  disabled,
}: {
  landing: Landing;
  onEditTemplate: () => void;
  disabled: boolean;
}) {
  const { t } = useTranslation("landings");
  return (
    <div className="flex flex-col gap-3 rounded-md border bg-muted/30 p-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="grid gap-1 text-xs">
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
          <span>
            <span className="text-muted-foreground">{t("bundle.entryFile")}:</span> <Mono>{landing.entryFile}</Mono>
          </span>
          <span>
            <span className="text-muted-foreground">{t("bundle.files")}:</span>{" "}
            <span className="tabular-nums">{landing.fileCount}</span>
          </span>
          <span>
            <span className="text-muted-foreground">{t("bundle.size")}:</span>{" "}
            <span className="tabular-nums">{formatBytes(landing.bundleSize)}</span>
          </span>
        </div>
        {landing.url && (
          <div className="truncate">
            <span className="text-muted-foreground">{t("bundle.assetsUrl")}:</span> <Mono className="text-xs">{landing.url}</Mono>
          </div>
        )}
      </div>
      <Button type="button" variant="outline" size="sm" onClick={onEditTemplate} disabled={disabled}>
        <CodeIcon className="size-3.5" />
        {t("rowActions.editTemplate")}
      </Button>
    </div>
  );
}

function ArchiveDropzone({
  file,
  onChange,
  label,
  invalid,
  disabled,
}: {
  file: File | null;
  onChange: (file: File | null) => void;
  label: string;
  invalid: boolean;
  disabled: boolean;
}) {
  const { t } = useTranslation("landings");
  const inputRef = React.useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = React.useState(false);

  return (
    <div className="grid gap-1.5">
      <Label id="lnd-archive-label">{label}</Label>
      <div className="relative">
        <button
          type="button"
          aria-labelledby="lnd-archive-label"
          disabled={disabled}
          onClick={() => inputRef.current?.click()}
          onDragOver={(e) => {
            e.preventDefault();
            setDragging(true);
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={(e) => {
            e.preventDefault();
            setDragging(false);
            const dropped = e.dataTransfer.files?.[0];
            if (dropped) onChange(dropped);
          }}
          className={cn(
            "flex w-full items-center gap-3 rounded-lg border-2 border-dashed px-4 py-5 text-left text-muted-foreground transition-colors",
            dragging
              ? "border-primary bg-primary/5 text-primary"
              : invalid
                ? "border-danger"
                : "border-border hover:border-muted-foreground/50 hover:text-foreground",
          )}
        >
          {file ? <FileArchiveIcon className="size-6 shrink-0" /> : <UploadCloudIcon className="size-6 shrink-0" />}
          <span className="grid min-w-0 gap-0.5">
            <span className="truncate text-sm font-medium text-foreground">{file ? file.name : t("form.archiveChoose")}</span>
            <span className="text-xs">{file ? formatBytes(file.size) : t("form.archiveHint")}</span>
          </span>
        </button>
        {file && !disabled && (
          <IconButton
            type="button"
            size="icon-xs"
            variant="secondary"
            className="absolute top-1/2 right-3 -translate-y-1/2 rounded-full"
            aria-label={t("form.archiveRemoveAria")}
            onClick={() => onChange(null)}
          >
            <XIcon className="size-3" />
          </IconButton>
        )}
        <input
          ref={inputRef}
          type="file"
          accept=".zip,application/zip,application/x-zip-compressed"
          className="hidden"
          onChange={(e) => {
            const picked = e.target.files?.[0];
            if (picked) onChange(picked);
            e.target.value = "";
          }}
        />
      </div>
    </div>
  );
}

/** The archive contract, spelled out where the operator uploads — what
 * apps/internal/landing/bundle.go and render.go actually enforce/inject. */
function ArchiveRequirements() {
  const { t } = useTranslation("landings");
  return (
    <details className="rounded-md border px-3 py-2 text-xs">
      <summary className="cursor-pointer font-medium">{t("requirements.title")}</summary>
      <ul className="mt-2 list-disc space-y-1 pl-4 text-muted-foreground">
        <li>{t("requirements.entry")}</li>
        <li>
          {t("requirements.relative")} <Mono>css/main.css</Mono>, <Mono>images/bg.jpg</Mono> —{" "}
          {t("requirements.notAbsolute")} <Mono>/style.css</Mono>.
        </li>
        <li>
          {t("requirements.trackUrl")} <Mono>trackUrl</Mono>, <Mono>engine_onClickButton()</Mono>.
        </li>
        <li>{t("requirements.links")}</li>
        <li>
          {t("requirements.manualBind")}{" "}
          <Mono>{"el.addEventListener('click', engine_onClickButton)"}</Mono>
        </li>
        <li>
          {t("requirements.twig")} <Mono>{'<a href="{{ trackUrl }}">'}</Mono>
        </li>
        <li>
          {t("requirements.skip")} <Mono>data-flox-skip</Mono>
        </li>
        <li>{t("requirements.limits")}</li>
      </ul>
    </details>
  );
}
