"use client";

import * as React from "react";
import { useForm, useWatch } from "react-hook-form";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { TriangleAlert } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useMe } from "@/hooks/use-auth";
import {
  useCancelOrganizationDeletion,
  useOrganization,
  useOrganizationDeletion,
  useScheduleOrganizationDeletion,
} from "@/hooks/use-organization";
import { ApiError } from "@/lib/api/client";
import { formatDeletionDate } from "@/lib/format";

type Values = { confirmName: string; password: string };

/** Danger zone: an owner schedules the organization's deletion — 7 days to
 * cancel, then everything goes (internal/orgdeletion, audit GDPR-1). Other
 * members see the date, not the buttons. */
export function OrganizationDeletionCard() {
  const { t, i18n } = useTranslation("settings");
  const me = useMe();
  const org = useOrganization();
  const deletion = useOrganizationDeletion();
  const schedule = useScheduleOrganizationDeletion();
  const cancel = useCancelOrganizationDeletion();
  const [open, setOpen] = React.useState(false);
  const form = useForm<Values>({ defaultValues: { confirmName: "", password: "" } });

  const isOwner = me.data?.role === "Owner";
  const name = org.data?.name ?? "";
  const scheduledAt = deletion.data?.scheduledAt ?? null;
  const [typed, password] = useWatch({ control: form.control, name: ["confirmName", "password"] });

  function onSubmit(values: Values) {
    schedule.mutate(values, {
      onSuccess: () => {
        setOpen(false);
        form.reset();
        toast(t("deletion.scheduledToast"));
      },
      onError: (err) => {
        const fields = err instanceof ApiError ? err.fields : undefined;
        if (fields?.password) {
          form.setError("password", { message: t("deletion.wrongPassword") });
        } else if (fields?.confirmName) {
          form.setError("confirmName", { message: t("deletion.wrongName") });
        } else {
          toast.error(t("deletion.scheduleError"), { description: err.message });
        }
      },
    });
  }

  function onCancel() {
    cancel.mutate(undefined, {
      onSuccess: () => toast(t("deletion.cancelledToast")),
      onError: (err) => toast.error(t("deletion.cancelError"), { description: err.message }),
    });
  }

  if (!deletion.data || !org.data) return null;

  return (
    <Card className="border-danger/40">
      <CardHeader>
        <CardTitle className="text-danger">{t("deletion.title")}</CardTitle>
        <CardDescription>{t("deletion.description")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {scheduledAt ? (
          <>
            <p className="flex items-center gap-2 text-sm">
              <TriangleAlert aria-hidden="true" className="size-4 text-danger" />
              {t("deletion.scheduledFor", { date: formatDeletionDate(scheduledAt, i18n.language), by: deletion.data.requestedBy })}
            </p>
            {isOwner && (
              <Button variant="outline" className="w-fit" onClick={onCancel} disabled={cancel.isPending}>
                {t("deletion.cancel")}
              </Button>
            )}
          </>
        ) : isOwner ? (
          <Button variant="destructive" className="w-fit" onClick={() => setOpen(true)}>
            {t("deletion.open")}
          </Button>
        ) : (
          <p className="text-sm text-muted-foreground">{t("deletion.ownerOnly")}</p>
        )}
      </CardContent>

      <Dialog
        open={open}
        onOpenChange={(v) => {
          setOpen(v);
          if (!v) form.reset();
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("deletion.confirmTitle", { name })}</DialogTitle>
            <DialogDescription>{t("deletion.confirmDescription")}</DialogDescription>
          </DialogHeader>
          <form onSubmit={form.handleSubmit(onSubmit)} className="flex flex-col gap-4">
            <div className="grid gap-1.5">
              <Label htmlFor="delete-org-name">{t("deletion.typeName", { name })}</Label>
              <Input id="delete-org-name" autoComplete="off" {...form.register("confirmName")} aria-invalid={!!form.formState.errors.confirmName} />
              {form.formState.errors.confirmName && (
                <p className="text-xs text-danger">{form.formState.errors.confirmName.message}</p>
              )}
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="delete-org-password">{t("deletion.password")}</Label>
              <Input
                id="delete-org-password"
                type="password"
                autoComplete="current-password"
                {...form.register("password")}
                aria-invalid={!!form.formState.errors.password}
              />
              {form.formState.errors.password && (
                <p className="text-xs text-danger">{form.formState.errors.password.message}</p>
              )}
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                {t("common:actions.cancel")}
              </Button>
              <Button type="submit" variant="destructive" disabled={typed.trim() !== name || !password || schedule.isPending}>
                {t("deletion.confirm")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
