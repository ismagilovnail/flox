import type { LucideIcon } from "lucide-react";
import { BellIcon, CheckCircle2Icon, LayoutTemplateIcon, MousePointerClickIcon, SendIcon, SmartphoneIcon } from "lucide-react";
import { format } from "date-fns";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { Card } from "@/components/ui/card";
import { Caption, Mono } from "@/components/ui/typography";
import type { CpaStatus, EventType, TimelineEvent } from "@/lib/api/conversions";

/** Real event types (§43, ~20 of them) grouped into the same six icon
 * buckets the old fixed-stage mock used, but the label under each icon is
 * the real event type, not a synthesized sentence — this timeline shows
 * whatever actually happened for a click_id, in whatever order, not a
 * fixed six-item funnel every conversion gets forced into. */
function iconFor(type: EventType): LucideIcon {
  if (type === "SOURCE_CLICK" || type === "SOURCE_FILTER") return MousePointerClickIcon;
  if (type.startsWith("LAND_") || type.startsWith("POSTLANDING_")) return LayoutTemplateIcon;
  if (type.startsWith("PWA_") || type === "IOS_INSTALL") return SmartphoneIcon;
  if (type.startsWith("NOTIFICATION_")) return BellIcon;
  if (type.startsWith("TG_")) return SendIcon;
  return CheckCircle2Icon; // CPA_*
}

const CPA_LABEL_I18N_KEY: Record<CpaStatus, string> = {
  CPA_HOLD: "timeline.cpaLabel.hold",
  CPA_ACCEPT: "timeline.cpaLabel.accept",
  CPA_REDEP: "timeline.cpaLabel.redep",
  CPA_DECLINE: "timeline.cpaLabel.decline",
  CPA_TRASH: "timeline.cpaLabel.trash",
};

function labelFor(type: EventType, t: TFunction): string {
  if (type in CPA_LABEL_I18N_KEY) return t(CPA_LABEL_I18N_KEY[type as CpaStatus], { ns: "conversions" });
  return type
    .toLowerCase()
    .split("_")
    .map((w) => w[0].toUpperCase() + w.slice(1))
    .join(" ");
}

function Connector() {
  return <div className="ml-[15px] h-4 w-px bg-border" />;
}

export function ConversionTimeline({ events }: { events: TimelineEvent[] }) {
  const { t } = useTranslation("conversions");
  return (
    <div className="flex flex-col">
      {events.map((event, i) => {
        const Icon = iconFor(event.type);
        return (
          <div key={`${event.type}-${event.eventAt}`}>
            <Card size="sm" className="flex-row items-center gap-3 px-3 py-2.5">
              <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-muted">
                <Icon className="size-4 text-muted-foreground" />
              </span>
              <div className="flex min-w-0 flex-1 flex-col">
                <div className="flex items-center justify-between gap-2">
                  <span className="text-sm font-medium">{labelFor(event.type, t)}</span>
                  <Mono className="shrink-0 text-xs text-muted-foreground">
                    {format(new Date(event.eventAt), "MMM d, HH:mm:ss")}
                  </Mono>
                </div>
                {event.isConversion && (
                  <Caption className="text-muted-foreground/80">
                    {event.hasUsdValue ? `${(event.revenue ?? 0).toFixed(2)} ${event.currency}` : t("timeline.noRevenue")}
                  </Caption>
                )}
              </div>
            </Card>
            {i < events.length - 1 && <Connector />}
          </div>
        );
      })}
    </div>
  );
}
