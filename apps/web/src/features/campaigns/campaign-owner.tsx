"use client";

import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { UserRound } from "lucide-react";

import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useMe, usePermission } from "@/hooks/use-auth";
import { useBuyerNames } from "@/hooks/use-buyers";
import { useSetCampaignOwner } from "@/hooks/use-campaigns";
import { useTeamMembers } from "@/hooks/use-team";
import { useApiErrorMessage } from "@/hooks/use-api-error-message";
import type { Campaign } from "@/lib/api/campaigns";

const NOBODY = "__nobody__";

/** Who a campaign belongs to (audit TEAM-1). Members with
 * campaign.manage_all pick the owner among the team's active members;
 * everyone else just reads it. */
export function CampaignOwner({ campaign }: { campaign: Campaign }) {
  const { t } = useTranslation("campaigns");
  const errorMessage = useApiErrorMessage();
  const me = useMe().data;
  const canAssign = usePermission("campaign.manage_all");
  const members = useTeamMembers({ enabled: canAssign }).data ?? [];
  const buyers = useBuyerNames().data?.buyers ?? [];
  const setOwner = useSetCampaignOwner(campaign.id);

  const owner = campaign.ownerUserId;
  const ownerName =
    owner === null
      ? t("owner.nobody")
      : owner === me?.user.id
        ? t("owner.you")
        : (members.find((m) => m.userId === owner)?.name ?? buyers.find((b) => b.id === owner)?.name ?? t("owner.unknown"));

  if (!canAssign) {
    return (
      <span className="flex items-center gap-1.5 text-sm text-muted-foreground">
        <UserRound aria-hidden="true" className="size-3.5" />
        {t("owner.label")}: <span className="text-foreground">{ownerName}</span>
      </span>
    );
  }

  const active = members.filter((m) => m.status === "active");
  return (
    <Select
      value={owner ?? NOBODY}
      disabled={setOwner.isPending}
      onValueChange={(v) =>
        setOwner.mutate(v === NOBODY ? "" : v, {
          onSuccess: () => toast(t("owner.changed")),
          onError: (err) => toast.error(t("owner.changeError"), { description: errorMessage(err) }),
        })
      }
    >
      <SelectTrigger size="sm" className="w-64" aria-label={t("owner.label")}>
        <UserRound aria-hidden="true" className="size-3.5 text-muted-foreground" />
        <span className="text-muted-foreground">{t("owner.label")}</span>
        <SelectValue placeholder={ownerName} />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={NOBODY}>{t("owner.nobody")}</SelectItem>
        {active.map((m) => (
          <SelectItem key={m.userId} value={m.userId}>
            {m.name} · {m.role}
          </SelectItem>
        ))}
        {owner !== null && !active.some((m) => m.userId === owner) && <SelectItem value={owner}>{ownerName}</SelectItem>}
      </SelectContent>
    </Select>
  );
}
