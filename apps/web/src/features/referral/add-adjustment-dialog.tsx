"use client";

import * as React from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useReferralStore } from "@/stores/referral";

export function AddAdjustmentDialog({
  open,
  onOpenChange,
  currentMemberId,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  currentMemberId: string;
}) {
  const addAdjustment = useReferralStore((s) => s.addAdjustment);
  const [amount, setAmount] = React.useState("");
  const [description, setDescription] = React.useState("");

  const parsed = Number(amount);
  const isValid = Number.isFinite(parsed) && parsed !== 0 && description.trim().length > 0;

  function submit() {
    if (!isValid) return;
    if (addAdjustment(parsed, description, currentMemberId)) {
      toast("Adjustment recorded", { description });
      onOpenChange(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add a manual adjustment</DialogTitle>
          <DialogDescription>
            Positive credits or negative corrections to the balance — every adjustment is a permanent, reasoned
            ledger entry (§54), never a silent edit.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-1.5">
          <Label htmlFor="adj-amount">Amount (USD, negative for a correction)</Label>
          <Input id="adj-amount" type="number" step="0.01" value={amount} onChange={(e) => setAmount(e.target.value)} placeholder="25.00" />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="adj-desc">Reason</Label>
          <Textarea
            id="adj-desc"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Why is this adjustment being made?"
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={submit} disabled={!isValid}>
            Add adjustment
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
