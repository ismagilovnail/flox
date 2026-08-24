"use client";

import { SearchIcon } from "lucide-react";
import { useTranslation } from "react-i18next";

import { MobileNav } from "@/components/shell/mobile-nav";
import { ShellBreadcrumbs } from "@/components/shell/breadcrumbs";
import { CommandMenu } from "@/components/shell/command-menu";
import { Notifications } from "@/components/shell/notifications";
import { UserMenu } from "@/components/shell/user-menu";
import { ThemeToggle } from "@/components/theme-toggle";
import { LanguageSwitcher } from "@/components/language-switcher";
import { Button } from "@/components/ui/button";
import { IconButton } from "@/components/ui/icon-button";
import { useCommandMenuStore } from "@/stores/command-menu";

export function Topbar() {
  const setOpen = useCommandMenuStore((s) => s.setOpen);
  const { t } = useTranslation("nav");

  return (
    <header className="sticky top-0 z-20 flex h-12 shrink-0 items-center gap-3 border-b border-border bg-background/95 px-3 backdrop-blur">
      <MobileNav />
      <div className="min-w-0 flex-1">
        <ShellBreadcrumbs />
      </div>

      <div className="flex shrink-0 items-center gap-1.5">
        <Button
          variant="outline"
          size="sm"
          className="hidden w-56 justify-start gap-2 text-muted-foreground sm:flex"
          onClick={() => setOpen(true)}
        >
          <SearchIcon className="size-3.5" />
          <span className="flex-1 text-left">{t("topbar.searchPlaceholder")}</span>
          <kbd className="rounded border border-border bg-muted px-1 font-mono text-[0.6875rem]">
            ⌘K
          </kbd>
        </Button>
        <IconButton
          aria-label={t("topbar.searchAria")}
          variant="outline"
          className="sm:hidden"
          onClick={() => setOpen(true)}
        >
          <SearchIcon className="size-4" />
        </IconButton>
        <Notifications />
        <LanguageSwitcher />
        <ThemeToggle />
        <UserMenu />
      </div>

      <CommandMenu />
    </header>
  );
}
