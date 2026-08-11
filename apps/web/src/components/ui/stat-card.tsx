import * as React from "react"
import { ArrowDownIcon, ArrowUpIcon, MinusIcon } from "lucide-react"

import { cn } from "@/lib/utils"
import { Card } from "@/components/ui/card"

type Trend = "up" | "down" | "flat"

/** Trend polarity: for metrics like CPA, "up" is bad, so callers pass the semantic direction, not the raw sign. */
type TrendTone = "positive" | "negative" | "neutral"

function trendTone(trend: Trend, direction: "up-is-good" | "up-is-bad") {
  if (trend === "flat") return "neutral" satisfies TrendTone
  const isUp = trend === "up"
  const good = direction === "up-is-good" ? isUp : !isUp
  return (good ? "positive" : "negative") satisfies TrendTone
}

const toneClasses: Record<TrendTone, string> = {
  positive: "text-success",
  negative: "text-danger",
  neutral: "text-muted-foreground",
}

function StatCard({
  className,
  label,
  value,
  delta,
  trend,
  direction = "up-is-good",
  ...props
}: React.ComponentProps<"div"> & {
  label: string
  value: React.ReactNode
  delta?: string
  trend?: Trend
  direction?: "up-is-good" | "up-is-bad"
}) {
  const tone = trend ? trendTone(trend, direction) : undefined
  const TrendIcon = trend === "up" ? ArrowUpIcon : trend === "down" ? ArrowDownIcon : MinusIcon

  return (
    <Card
      data-slot="stat-card"
      className={cn("gap-1.5 px-4 py-3", className)}
      {...props}
    >
      <span className="text-xs text-muted-foreground">{label}</span>
      <div className="flex flex-wrap items-baseline justify-between gap-x-2 gap-y-0.5">
        <span className="font-mono text-2xl font-semibold font-tabular text-foreground">
          {value}
        </span>
        {delta && tone && (
          <span
            className={cn(
              "flex shrink-0 items-center gap-0.5 text-xs font-medium font-tabular",
              toneClasses[tone],
            )}
          >
            <TrendIcon className="size-3" />
            {delta}
          </span>
        )}
      </div>
    </Card>
  )
}

export { StatCard }
