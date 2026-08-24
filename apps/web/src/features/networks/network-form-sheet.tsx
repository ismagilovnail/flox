"use client";

import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Switch } from "@/components/ui/switch";
import { MacroPicker } from "@/components/shared/macro-picker";
import type { NetworkStatus } from "@/lib/api/networks";

/** Factory, not a module-level const — see source-form-sheet.tsx's
 * buildSourceFormSchema for why (Zod messages are user-facing text and
 * need the live translator). */
export function buildNetworkFormSchema(t: TFunction) {
  return z.object({
    name: z.string().min(2, t("form.validation.nameMin", { ns: "networks" })).max(80),
    postbackUrl: z.url(t("form.validation.urlInvalid", { ns: "networks" })),
    acceptDuplicates: z.boolean(),
    status: z.enum(["active", "paused", "archived"] as [NetworkStatus, ...NetworkStatus[]]).optional(),
  });
}

export type NetworkFormValues = z.infer<ReturnType<typeof buildNetworkFormSchema>>;

const STATUS_OPTIONS: NetworkStatus[] = ["active", "paused", "archived"];

export function NetworkFormSheet({
  open,
  onOpenChange,
  defaultValues,
  title,
  submitLabel,
  showStatus = false,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  defaultValues: Partial<NetworkFormValues>;
  title: string;
  submitLabel: string;
  showStatus?: boolean;
  onSubmit: (values: NetworkFormValues) => void;
}) {
  const { t } = useTranslation(["networks", "common"]);
  const form = useForm<NetworkFormValues>({
    resolver: zodResolver(buildNetworkFormSchema(t)),
    values: { name: "", postbackUrl: "", acceptDuplicates: false, status: "active", ...defaultValues },
  });

  const {
    register,
    handleSubmit,
    control,
    setValue,
    getValues,
    formState: { errors, isSubmitting },
  } = form;

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full gap-0 overflow-y-auto sm:max-w-lg" side="right">
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription>{t("form.description", { ns: "networks" })}</SheetDescription>
        </SheetHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="flex flex-col gap-4 px-4 pb-4">
          <div className="grid gap-1.5">
            <Label htmlFor="net-name">{t("form.nameLabel", { ns: "networks" })}</Label>
            <Input
              id="net-name"
              placeholder={t("form.namePlaceholder", { ns: "networks" })}
              {...register("name")}
              aria-invalid={!!errors.name}
            />
            {errors.name && <p className="text-xs text-danger">{errors.name.message}</p>}
          </div>

          <div className="grid gap-1.5">
            <div className="flex items-center justify-between">
              <Label htmlFor="net-postback">{t("form.postbackUrlLabel", { ns: "networks" })}</Label>
              <MacroPicker onInsert={(token) => setValue("postbackUrl", `${getValues("postbackUrl")}${token}`)} />
            </div>
            <Input
              id="net-postback"
              placeholder={t("form.postbackUrlPlaceholder", { ns: "networks" })}
              className="font-mono text-xs"
              {...register("postbackUrl")}
              aria-invalid={!!errors.postbackUrl}
            />
            {errors.postbackUrl && <p className="text-xs text-danger">{errors.postbackUrl.message}</p>}
            <p className="text-xs text-muted-foreground">{t("form.postbackUrlHint", { ns: "networks" })}</p>
          </div>

          <div className="flex items-center justify-between rounded-md border border-border p-2.5">
            <div>
              <p className="text-sm font-medium">{t("form.acceptDuplicatesLabel", { ns: "networks" })}</p>
              <p className="text-xs text-muted-foreground">{t("form.acceptDuplicatesHint", { ns: "networks" })}</p>
            </div>
            <Controller
              control={control}
              name="acceptDuplicates"
              render={({ field }) => (
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                  aria-label={t("form.acceptDuplicatesAria", { ns: "networks" })}
                />
              )}
            />
          </div>

          {showStatus && (
            <div className="grid gap-1.5">
              <Label htmlFor="net-status">{t("form.statusLabel", { ns: "networks" })}</Label>
              <Controller
                control={control}
                name="status"
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger id="net-status" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {STATUS_OPTIONS.map((s) => (
                        <SelectItem key={s} value={s}>
                          {t(`status.${s}`, { ns: "common" })}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            </div>
          )}

          <SheetFooter className="mt-0 flex-row justify-end gap-2 p-0">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              {t("actions.cancel", { ns: "common" })}
            </Button>
            <Button type="submit" disabled={isSubmitting}>
              {submitLabel}
            </Button>
          </SheetFooter>
        </form>
      </SheetContent>
    </Sheet>
  );
}
