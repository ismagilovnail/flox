import * as React from "react"
import { AlertTriangleIcon } from "lucide-react"

import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"

function ErrorState({
  className,
  title = "Something went wrong",
  description,
  onRetry,
  ...props
}: React.ComponentProps<"div"> & {
  title?: string
  description?: string
  onRetry?: () => void
}) {
  return (
    <div
      data-slot="error-state"
      className={cn(
        "flex flex-col items-center justify-center gap-3 rounded-lg border border-danger/30 bg-danger/5 px-6 py-12 text-center",
        className,
      )}
      {...props}
    >
      <div className="flex size-10 items-center justify-center rounded-full bg-danger/10">
        <AlertTriangleIcon className="size-5 text-danger" />
      </div>
      <div className="flex flex-col gap-1">
        <p className="text-sm font-medium text-foreground">{title}</p>
        {description && (
          <p className="max-w-sm text-sm text-muted-foreground">
            {description}
          </p>
        )}
      </div>
      {onRetry && (
        <Button variant="outline" size="sm" onClick={onRetry}>
          Retry
        </Button>
      )}
    </div>
  )
}

export { ErrorState }
