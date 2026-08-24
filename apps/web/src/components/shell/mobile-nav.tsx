"use client";

import * as React from "react";
import { MenuIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { IconButton } from "@/components/ui/icon-button";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { NavContent } from "@/components/shell/nav-content";

export function MobileNav() {
  const [open, setOpen] = React.useState(false);
  const { t } = useTranslation("nav");

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>
        <IconButton aria-label={t("sidebar.openNavigationAria")} variant="ghost" className="md:hidden">
          <MenuIcon className="size-4" />
        </IconButton>
      </SheetTrigger>
      <SheetContent side="left" className="w-72 p-0">
        <SheetHeader className="sr-only">
          <SheetTitle>{t("sidebar.navigationTitle")}</SheetTitle>
        </SheetHeader>
        <NavContent onNavigate={() => setOpen(false)} />
      </SheetContent>
    </Sheet>
  );
}
