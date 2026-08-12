import type { LucideIcon } from "lucide-react";
import { CheckCircle2Icon, MousePointerClickIcon, LayoutTemplateIcon, SendIcon, SmartphoneIcon, TargetIcon } from "lucide-react";
import { format } from "date-fns";

import { Card } from "@/components/ui/card";
import { Caption, Mono } from "@/components/ui/typography";
import type { TimelineStage, TimelineStep } from "@/lib/mock/conversions";

const STAGE_ICON: Record<TimelineStage, LucideIcon> = {
  Click: MousePointerClickIcon,
  Landing: LayoutTemplateIcon,
  PWA: SmartphoneIcon,
  Offer: TargetIcon,
  Conversion: CheckCircle2Icon,
  Postback: SendIcon,
};

function Connector() {
  return <div className="ml-[15px] h-4 w-px bg-border" />;
}

export function ConversionTimeline({ steps }: { steps: TimelineStep[] }) {
  return (
    <div className="flex flex-col">
      {steps.map((step, i) => {
        const Icon = STAGE_ICON[step.stage];
        return (
          <div key={step.stage}>
            <Card size="sm" className="flex-row items-center gap-3 px-3 py-2.5">
              <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-muted">
                <Icon className="size-4 text-muted-foreground" />
              </span>
              <div className="flex min-w-0 flex-1 flex-col">
                <div className="flex items-center justify-between gap-2">
                  <span className="text-sm font-medium">{step.stage}</span>
                  <Mono className="shrink-0 text-xs text-muted-foreground">
                    {format(new Date(step.timestamp), "MMM d, HH:mm:ss")}
                  </Mono>
                </div>
                <Caption>{step.label}</Caption>
                {step.description && <Caption className="text-muted-foreground/80">{step.description}</Caption>}
              </div>
            </Card>
            {i < steps.length - 1 && <Connector />}
          </div>
        );
      })}
    </div>
  );
}
