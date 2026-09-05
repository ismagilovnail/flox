"use client";

import * as React from "react";
import { toast } from "sonner";
import { BanknoteIcon, CheckIcon, XIcon } from "lucide-react";

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
  const approvePayout = useApprovePayout();
  const rejectPayout = useRejectPayout();
  const markPayoutPaid = useMarkPayoutPaid();

  const [rejecting, setRejecting] = React.useState(false);
  const [note, setNote] = React.useState("");

  function approve() {
    approvePayout.mutate(payout.id, {
      onSuccess: () => toast("Payout approved"),
      onError: (err) => toast.error("Couldn't approve payout", { description: err.message }),
    });
  }

  function markPaid() {
    markPayoutPaid.mutate(payout.id, {
      onSuccess: () => toast("Payout marked as paid"),
      onError: (err) => toast.error("Couldn't mark payout as paid", { description: err.message }),
    });
  }

  function submitReject() {
    rejectPayout.mutate(
      { id: payout.id, note: note.trim() },
      {
        onSuccess: () => {
          setRejecting(false);
          setNote("");
          toast("Payout rejected");
        },
        onError: (err) => toast.error("Couldn't reject payout", { description: err.message }),
      },
    );
  }

  if (payout.status === "pending") {
    return (
      <>
        <div className="flex justify-end gap-1">
          <Button size="sm" variant="outline" onClick={approve}>
            <CheckIcon className="size-3.5" /> Approve
          </Button>
          <IconButton aria-label="Reject payout" size="icon-sm" onClick={() => setRejecting(true)}>
            <XIcon className="size-3.5" />
          </IconButton>
        </div>

        <Dialog open={rejecting} onOpenChange={setRejecting}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Reject payout of {payout.amount.toFixed(2)} USD?</DialogTitle>
              <DialogDescription>The requester can submit a new request once the balance allows it.</DialogDescription>
            </DialogHeader>
            <div className="grid gap-1.5">
              <Label htmlFor="reject-note">Reason</Label>
              <Textarea id="reject-note" value={note} onChange={(e) => setNote(e.target.value)} placeholder="Why?" />
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setRejecting(false)}>
                Cancel
              </Button>
              <Button variant="destructive" onClick={submitReject} disabled={!note.trim()}>
                Reject
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
          <BanknoteIcon className="size-3.5" /> Mark as paid
        </Button>
      </div>
    );
  }

  return null;
}
