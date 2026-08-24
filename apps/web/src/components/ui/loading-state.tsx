"use client"

import * as React from "react"
import { Loader2Icon } from "lucide-react"
import { useTranslation } from "react-i18next"

import { cn } from "@/lib/utils"

function LoadingState({
  className,
  label,
  ...props
}: React.ComponentProps<"div"> & { label?: string }) {
  const { t } = useTranslation("common")
  return (
    <div
      data-slot="loading-state"
      role="status"
      className={cn(
        "flex flex-col items-center justify-center gap-2 px-6 py-12 text-center",
        className,
      )}
      {...props}
    >
      <Loader2Icon className="size-5 animate-spin text-muted-foreground" />
      <span className="text-sm text-muted-foreground">{label ?? t("states.loading")}</span>
    </div>
  )
}

export { LoadingState }
