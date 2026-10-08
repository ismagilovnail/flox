"use client";

import * as React from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

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
  const { t } = useTranslation(["referral", "common"]);
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
        toast(t("payout.requested"), { description: formatUsd(parsed, 2) });
        onOpenChange(false);
      },
      onError: (err) => toast.error(t("payout.requestError"), { description: err.message }),
    });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("payout.title")}</DialogTitle>
          <DialogDescription>{t("payout.description", { balance: formatUsd(availableBalance, 2) })}</DialogDescription>
        </DialogHeader>
        <div className="grid gap-1.5">
          <Label htmlFor="payout-amount">{t("payout.amount")}</Label>
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
          {!isValid && <p className="text-xs text-danger">{t("payout.amountError", { max: formatUsd(availableBalance, 2) })}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common:actions.cancel")}
          </Button>
          <Button onClick={submit} disabled={!isValid || requestPayout.isPending}>
            {t("payout.request")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
