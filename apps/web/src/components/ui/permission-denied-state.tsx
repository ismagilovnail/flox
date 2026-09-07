"use client"

import * as React from "react"
import { LockIcon } from "lucide-react"
import { useTranslation } from "react-i18next"

import { cn } from "@/lib/utils"

function PermissionDeniedState({
  className,
  title,
  description,
  ...props
}: React.ComponentProps<"div"> & {
  title?: string
  description?: string
}) {
  const { t } = useTranslation("common")
  return (
    <div
      data-slot="permission-denied-state"
      className={cn(
        "flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-border px-6 py-12 text-center",
        className,
      )}
      {...props}
    >
      <div className="flex size-10 items-center justify-center rounded-full bg-muted">
        <LockIcon className="size-5 text-muted-foreground" />
      </div>
      <div className="flex flex-col gap-1">
        <p className="text-sm font-medium text-foreground">{title ?? t("permissionDenied.title")}</p>
        <p className="max-w-sm text-sm text-muted-foreground">{description ?? t("permissionDenied.description")}</p>
      </div>
    </div>
  )
}

export { PermissionDeniedState }
