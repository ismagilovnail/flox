"use client";

import * as React from "react";
import { toast } from "sonner";
import { BanknoteIcon, CheckIcon, XIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { Button } from "@/components/ui/button";
import { IconButton } from "@/components/ui/icon-button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useApprovePayout, useMarkPayoutPaid, useRejectPayout } from "@/hooks/use-referral";
import type { PayoutRequest } from "@/lib/referral";

export function PayoutRowActions({ payout }: { payout: PayoutRequest }) {
  const { t } = useTranslation(["referral", "common"]);
  const approvePayout = useApprovePayout();
  const rejectPayout = useRejectPayout();
  const markPayoutPaid = useMarkPayoutPaid();

  const [rejecting, setRejecting] = React.useState(false);
  const [note, setNote] = React.useState("");

  function approve() {
    approvePayout.mutate(payout.id, {
      onSuccess: () => toast(t("payouts.approved")),
      onError: (err) => toast.error(t("payouts.approveError"), { description: err.message }),
    });
  }

  function markPaid() {
    markPayoutPaid.mutate(payout.id, {
      onSuccess: () => toast(t("payouts.markedPaid")),
      onError: (err) => toast.error(t("payouts.markPaidError"), { description: err.message }),
    });
  }

  function submitReject() {
    rejectPayout.mutate(
      { id: payout.id, note: note.trim() },
      {
        onSuccess: () => {
          setRejecting(false);
          setNote("");
          toast(t("payouts.rejected"));
        },
        onError: (err) => toast.error(t("payouts.rejectError"), { description: err.message }),
      },
    );
  }

  if (payout.status === "pending") {
    return (
      <>
        <div className="flex justify-end gap-1">
          <Button size="sm" variant="outline" onClick={approve}>
            <CheckIcon className="size-3.5" /> {t("payouts.approve")}
          </Button>
          <IconButton aria-label={t("payouts.rejectAria")} size="icon-sm" onClick={() => setRejecting(true)}>
            <XIcon className="size-3.5" />
          </IconButton>
        </div>

        <Dialog open={rejecting} onOpenChange={setRejecting}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t("payouts.rejectTitle", { amount: payout.amount.toFixed(2) })}</DialogTitle>
              <DialogDescription>{t("payouts.rejectDescription")}</DialogDescription>
            </DialogHeader>
            <div className="grid gap-1.5">
              <Label htmlFor="reject-note">{t("payouts.reason")}</Label>
              <Textarea id="reject-note" value={note} onChange={(e) => setNote(e.target.value)} placeholder={t("payouts.reasonPlaceholder")} />
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setRejecting(false)}>
                {t("common:actions.cancel")}
              </Button>
              <Button variant="destructive" onClick={submitReject} disabled={!note.trim()}>
                {t("payouts.reject")}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </>
    );
  }

  if (payout.status === "approved") {
    return (
      <div className="flex justify-end">
        <Button size="sm" variant="outline" onClick={markPaid}>
          <BanknoteIcon className="size-3.5" /> {t("payouts.markPaid")}
        </Button>
      </div>
    );
  }

  return null;
}
