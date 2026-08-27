"use client";

import * as React from "react";
import Link from "next/link";
import { toast } from "sonner";
import { BarChart3Icon, CopyIcon, KeyRoundIcon, MoreHorizontalIcon, PauseIcon, PencilIcon, PlayIcon, Trash2Icon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { IconButton } from "@/components/ui/icon-button";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
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
import {
  useActivateNetwork,
  useArchiveNetwork,
  useDuplicateNetwork,
  usePauseNetwork,
  useRegeneratePostbackSecret,
} from "@/hooks/use-networks";
import { viewStatisticsHref } from "@/features/analytics/view-statistics-link";
import { PostbackSecretDialog } from "@/features/networks/network-list";
import type { Network } from "@/lib/api/networks";

export function NetworkRowActions({ network, onEdit }: { network: Network; onEdit: () => void }) {
  const { t } = useTranslation(["networks", "common"]);
  const pause = usePauseNetwork();
  const activate = useActivateNetwork();
  const duplicate = useDuplicateNetwork();
  const archive = useArchiveNetwork();
  const regenerateSecret = useRegeneratePostbackSecret();
  const [confirmArchive, setConfirmArchive] = React.useState(false);
  const [revealedSecret, setRevealedSecret] = React.useState<string | null>(null);

  function handleRegenerateSecret() {
    regenerateSecret.mutate(network.id, {
      onSuccess: (result) => setRevealedSecret(result.postbackSecret),
      onError: (err) => toast.error(t("postbackSecret.regenerateError"), { description: err.message }),
    });
  }

  function togglePause() {
    const action = network.status === "active" ? pause : activate;
    action.mutate(network.id, {
      onSuccess: () => toast(t(network.status === "active" ? "toast.paused" : "toast.resumed"), { description: network.name }),
      onError: (err) => toast.error(t("toast.updateError"), { description: err.message }),
    });
  }

  function handleDuplicate() {
    duplicate.mutate(network.id, {
      onSuccess: () => toast(t("toast.duplicated"), { description: t("toast.duplicatedSuffix", { name: network.name }) }),
      onError: (err) => toast.error(t("toast.duplicateError"), { description: err.message }),
    });
  }

  function handleArchive() {
    archive.mutate(network.id, {
      onSuccess: () => {
        setConfirmArchive(false);
        toast(t("toast.archived"), { description: network.name });
      },
      onError: (err) => {
        setConfirmArchive(false);
        toast.error(t("toast.archiveError"), { description: err.message });
      },
    });
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <IconButton aria-label={t("rowActions.actionsAria", { name: network.name })} variant="ghost" size="icon-sm">
            <MoreHorizontalIcon className="size-4" />
          </IconButton>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onSelect={onEdit}>
            <PencilIcon className="size-4" /> {t("rowActions.edit")}
          </DropdownMenuItem>
          <DropdownMenuItem asChild>
            <Link href={viewStatisticsHref("network", network.name)}>
              <BarChart3Icon className="size-4" /> {t("rowActions.viewStatistics")}
            </Link>
          </DropdownMenuItem>
          {network.status !== "archived" && (
            <DropdownMenuItem onSelect={togglePause}>
              {network.status === "active" ? (
                <>
                  <PauseIcon className="size-4" /> {t("rowActions.pause")}
                </>
              ) : (
                <>
                  <PlayIcon className="size-4" /> {t("rowActions.resume")}
                </>
              )}
            </DropdownMenuItem>
          )}
          <DropdownMenuItem onSelect={handleDuplicate}>
            <CopyIcon className="size-4" /> {t("rowActions.duplicate")}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={handleRegenerateSecret}>
            <KeyRoundIcon className="size-4" /> {t("rowActions.regenerateSecret")}
          </DropdownMenuItem>
          {network.status !== "archived" && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem variant="destructive" onSelect={() => setConfirmArchive(true)}>
                <Trash2Icon className="size-4" /> {t("rowActions.archive")}
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>

      <Dialog open={confirmArchive} onOpenChange={setConfirmArchive}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("rowActions.archiveConfirmTitle", { name: network.name })}</DialogTitle>
            <DialogDescription>{t("rowActions.archiveConfirmDescription")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmArchive(false)}>
              {t("actions.cancel", { ns: "common" })}
            </Button>
            <Button variant="destructive" onClick={handleArchive} disabled={archive.isPending}>
              {t("rowActions.archive")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {revealedSecret && (
        <PostbackSecretDialog networkId={network.id} secret={revealedSecret} onDone={() => setRevealedSecret(null)} />
      )}
    </>
  );
}
