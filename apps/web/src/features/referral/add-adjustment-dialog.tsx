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
import { Textarea } from "@/components/ui/textarea";
import { useAddTransaction } from "@/hooks/use-referral";

export function AddAdjustmentDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation(["referral", "common"]);
  const addTransaction = useAddTransaction();
  const [amount, setAmount] = React.useState("");
  const [description, setDescription] = React.useState("");

  const parsed = Number(amount);
  const isValid = Number.isFinite(parsed) && parsed !== 0 && description.trim().length > 0;

  function submit() {
    if (!isValid) return;
    addTransaction.mutate(
      { type: "adjustment", amount: parsed, description },
      {
        onSuccess: () => {
          toast(t("adjustment.recorded"), { description });
          onOpenChange(false);
        },
        onError: (err) => toast.error(t("adjustment.error"), { description: err.message }),
      },
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("adjustment.title")}</DialogTitle>
          <DialogDescription>{t("adjustment.description")}</DialogDescription>
        </DialogHeader>
        <div className="grid gap-1.5">
          <Label htmlFor="adj-amount">{t("adjustment.amount")}</Label>
          <Input id="adj-amount" type="number" step="0.01" value={amount} onChange={(e) => setAmount(e.target.value)} placeholder="25.00" />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="adj-desc">{t("adjustment.reason")}</Label>
          <Textarea
            id="adj-desc"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder={t("adjustment.reasonPlaceholder")}
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common:actions.cancel")}
          </Button>
          <Button onClick={submit} disabled={!isValid || addTransaction.isPending}>
            {t("adjustment.add")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
