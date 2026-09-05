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
import { formatUsd } from "@/lib/format";
import { useRequestPayout } from "@/hooks/use-referral";

export function RequestPayoutDialog({
  open,
  onOpenChange,
  availableBalance,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  availableBalance: number;
}) {
  const requestPayout = useRequestPayout();
  // The parent only renders this component while the dialog is open, so it
  // fully unmounts on close and remounts fresh on reopen — a lazy initializer
  // is enough (no effect needed to resync `amount` when `open` changes).
  const [amount, setAmount] = React.useState(() => availableBalance.toFixed(2));

  const parsed = Number(amount);
  const isValid = Number.isFinite(parsed) && parsed > 0 && parsed <= availableBalance;

  function submit() {
    if (!isValid) return;
    requestPayout.mutate(parsed, {
      onSuccess: () => {
        toast("Payout requested", { description: formatUsd(parsed, 2) });
        onOpenChange(false);
      },
      onError: (err) => toast.error("Couldn't request payout", { description: err.message }),
    });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Request a payout</DialogTitle>
          <DialogDescription>
            Available balance: {formatUsd(availableBalance, 2)}. Requests go through Owner/Admin approval before
            being marked paid (§30.7).
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-1.5">
          <Label htmlFor="payout-amount">Amount (USD)</Label>
          <Input
            id="payout-amount"
            type="number"
            step="0.01"
            min="0"
            max={availableBalance}
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            aria-invalid={!isValid}
          />
          {!isValid && <p className="text-xs text-danger">Enter an amount between 0.01 and {formatUsd(availableBalance, 2)}.</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={submit} disabled={!isValid || requestPayout.isPending}>
            Request payout
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
