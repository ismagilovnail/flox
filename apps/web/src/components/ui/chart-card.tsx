import * as React from "react"

import { cn } from "@/lib/utils"
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"

/** Chart chrome only — the chart library (Apache ECharts per stack) mounts inside `children`. */
function ChartCard({
  className,
  title,
  action,
  children,
  ...props
}: React.ComponentProps<"div"> & {
  title: string
  action?: React.ReactNode
}) {
  return (
    <Card data-slot="chart-card" className={cn(className)} {...props}>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        {action && <CardAction>{action}</CardAction>}
      </CardHeader>
      <CardContent className="h-64">{children}</CardContent>
    </Card>
  )
}

export { ChartCard }
