"use client";

import * as React from "react";
import Link from "next/link";
import { toast } from "sonner";
import { PencilIcon, Trash2Icon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { IconButton } from "@/components/ui/icon-button";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { usePermission } from "@/hooks/use-auth";
import { useDeleteDomain } from "@/hooks/use-domains";
import type { Domain } from "@/lib/api/domains";

export function DomainRowActions({ domain }: { domain: Domain }) {
  const { t } = useTranslation(["domains", "common"]);
  const canWrite = usePermission("settings.write");
  const deleteDomain = useDeleteDomain();
  const [confirmRemove, setConfirmRemove] = React.useState(false);

  function handleRemove() {
    deleteDomain.mutate(domain.id, {
      onSuccess: () => {
        setConfirmRemove(false);
        toast(t("actions.removed"), { description: domain.domain });
      },
      onError: (err) => {
        setConfirmRemove(false);
        toast.error(t("actions.removeError"), { description: err.message });
      },
    });
  }

  return (
    <>
      <div className="flex items-center justify-end gap-1">
        <Tooltip>
          <TooltipTrigger asChild>
            <IconButton aria-label={t("common:actions.edit")} size="icon-sm" asChild>
              <Link href={`/domains/${domain.id}`}>
                <PencilIcon className="size-3.5" />
              </Link>
            </IconButton>
          </TooltipTrigger>
          <TooltipContent>{t("common:actions.edit")}</TooltipContent>
        </Tooltip>

        {canWrite && (
          <Tooltip>
            <TooltipTrigger asChild>
              <IconButton aria-label={t("actions.remove")} size="icon-sm" onClick={() => setConfirmRemove(true)}>
                <Trash2Icon className="size-3.5" />
              </IconButton>
            </TooltipTrigger>
            <TooltipContent>{t("actions.remove")}</TooltipContent>
          </Tooltip>
        )}
      </div>

      <Dialog open={confirmRemove} onOpenChange={setConfirmRemove}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("actions.removeTitle", { domain: domain.domain })}</DialogTitle>
            <DialogDescription>{t("actions.removeDescription")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmRemove(false)}>
              {t("common:actions.cancel")}
            </Button>
            <Button variant="destructive" onClick={handleRemove} disabled={deleteDomain.isPending}>
              {t("actions.remove")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
