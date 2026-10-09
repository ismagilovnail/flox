"use client";

import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { BadgeCheckIcon, CopyIcon, SearchCheckIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { IconButton } from "@/components/ui/icon-button";
import { Caption, Mono } from "@/components/ui/typography";
import { usePermission } from "@/hooks/use-auth";
import { useVerifyDomainOwnership } from "@/hooks/use-domains";
import type { Domain } from "@/lib/api/domains";

/** Proving the organization controls the domain's DNS (tech audit T6): a
 * domain tracks only once verified. Until then this shows the TXT record
 * to add and a button that checks it; afterwards, when it was verified. */
export function DomainOwnershipCard({ domain }: { domain: Domain }) {
  const { t, i18n } = useTranslation("domains");
  const canWrite = usePermission("settings.write");
  const verify = useVerifyDomainOwnership();
  const verifiedAt = domain.ownershipVerifiedAt;

  function copy(value: string) {
    navigator.clipboard.writeText(value);
    toast(t("ownership.copied"));
  }

  function handleVerify() {
    verify.mutate(domain.id, {
      onSuccess: () => toast(t("ownership.verified"), { description: t("ownership.verifiedDescription", { domain: domain.domain }) }),
      onError: (err) => toast.error(t("ownership.notYet"), { description: err.message }),
    });
  }

  if (verifiedAt) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <BadgeCheckIcon className="size-4 text-success" /> {t("ownership.title")}
          </CardTitle>
          <CardDescription>
            {t("ownership.verifiedOn", { date: new Date(verifiedAt).toLocaleString(i18n.language) })}
          </CardDescription>
        </CardHeader>
      </Card>
    );
  }

  const record = domain.ownershipRecord;
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("ownership.title")}</CardTitle>
        <CardDescription>{t("ownership.instructions")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {[
          { label: t("ownership.type"), value: "TXT", copyable: false },
          { label: t("ownership.name"), value: record.name, copyable: true },
          { label: t("ownership.value"), value: record.value, copyable: true },
        ].map((row) => (
          <div key={row.label} className="flex flex-col gap-1">
            <Caption>{row.label}</Caption>
            <div className="flex items-center gap-2 rounded-md border border-border bg-muted px-3 py-2">
              <Mono className="min-w-0 flex-1 truncate text-xs">{row.value}</Mono>
              {row.copyable && (
                <IconButton aria-label={t("ownership.copyAria", { what: row.label })} size="icon-sm" onClick={() => copy(row.value)}>
                  <CopyIcon className="size-3.5" />
                </IconButton>
              )}
            </div>
          </div>
        ))}
        <div className="flex items-center gap-3">
          <Button type="button" size="sm" onClick={handleVerify} disabled={!canWrite || verify.isPending}>
            <SearchCheckIcon className="size-3.5" /> {verify.isPending ? t("ownership.checking") : t("ownership.check")}
          </Button>
          <Caption>{t("ownership.propagation")}</Caption>
        </div>
      </CardContent>
    </Card>
  );
}
