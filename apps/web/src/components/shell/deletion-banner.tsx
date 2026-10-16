"use client";

import Link from "next/link";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { TriangleAlert } from "lucide-react";

import { Button } from "@/components/ui/button";
import { useMe } from "@/hooks/use-auth";
import { useCancelOrganizationDeletion, useOrganizationDeletion } from "@/hooks/use-organization";
import { formatDeletionDate } from "@/lib/format";

/** On every page while the organization is scheduled for deletion: when,
 * and for an owner a way to cancel (audit GDPR-1). */
export function DeletionBanner() {
  const { t, i18n } = useTranslation("settings");
  const me = useMe();
  const deletion = useOrganizationDeletion();
  const cancel = useCancelOrganizationDeletion();
  const scheduledAt = deletion.data?.scheduledAt;
  if (!scheduledAt) return null;

  return (
    <div role="alert" className="flex flex-wrap items-center gap-3 border-b border-danger/40 bg-danger/10 px-6 py-2 text-sm">
      <TriangleAlert aria-hidden="true" className="size-4 text-danger" />
      <span>{t("deletion.banner", { date: formatDeletionDate(scheduledAt, i18n.language) })}</span>
      {me.data?.role === "Owner" ? (
        <Button
          size="sm"
          variant="outline"
          className="h-7"
          disabled={cancel.isPending}
          onClick={() =>
            cancel.mutate(undefined, {
              onSuccess: () => toast(t("deletion.cancelledToast")),
              onError: (err) => toast.error(t("deletion.cancelError"), { description: err.message }),
            })
          }
        >
          {t("deletion.cancel")}
        </Button>
      ) : (
        <Link href="/settings" className="font-medium text-primary underline-offset-4 hover:underline">
          {t("deletion.details")}
        </Link>
      )}
    </div>
  );
}
