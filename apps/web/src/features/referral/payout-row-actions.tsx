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
import { useReferralStore } from "@/stores/referral";
import type { PayoutRequest } from "@/lib/mock/referral";

export function PayoutRowActions({
  payout,
  currentMemberId,
}: {
  payout: PayoutRequest;
  currentMemberId: string;
}) {
  const approvePayout = useReferralStore((s) => s.approvePayout);
  const rejectPayout = useReferralStore((s) => s.rejectPayout);
  const markPayoutPaid = useReferralStore((s) => s.markPayoutPaid);

  const [rejecting, setRejecting] = React.useState(false);
  const [note, setNote] = React.useState("");

  function approve() {
    approvePayout(payout.id, currentMemberId);
    toast("Payout approved");
  }

  function markPaid() {
    markPayoutPaid(payout.id, currentMemberId);
    toast("Payout marked as paid");
  }

  function submitReject() {
    rejectPayout(payout.id, currentMemberId, note.trim());
    setRejecting(false);
    setNote("");
    toast("Payout rejected");
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
              <Button variant="destructive" onClick={submitReject}>
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
