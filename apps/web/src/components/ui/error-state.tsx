"use client"

import * as React from "react"
import { AlertTriangleIcon } from "lucide-react"
import { useTranslation } from "react-i18next"

import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"

function ErrorState({
  className,
  title,
  description,
  onRetry,
  ...props
}: React.ComponentProps<"div"> & {
  title?: string
  description?: string
  onRetry?: () => void
}) {
  const { t } = useTranslation("common")
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
        <p className="text-sm font-medium text-foreground">{title ?? t("states.errorTitle")}</p>
        {description && (
          <p className="max-w-sm text-sm text-muted-foreground">
            {description}
          </p>
        )}
      </div>
      {onRetry && (
        <Button variant="outline" size="sm" onClick={onRetry}>
          {t("actions.retry")}
        </Button>
      )}
    </div>
  )
}

export { ErrorState }
