"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useTranslation } from "react-i18next";

import { COMMAND_GROUPS } from "@/lib/nav";
import { useCommandMenuStore } from "@/stores/command-menu";
import {
  Command,
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";

export function CommandMenu() {
  const open = useCommandMenuStore((s) => s.open);
  const setOpen = useCommandMenuStore((s) => s.setOpen);
  const toggle = useCommandMenuStore((s) => s.toggle);
  const router = useRouter();
  const { t } = useTranslation("nav");

  React.useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "k" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        toggle();
      }
    }
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [toggle]);

  function go(href: string) {
    setOpen(false);
    router.push(href);
  }

  return (
    <CommandDialog open={open} onOpenChange={setOpen}>
      <Command>
        <CommandInput placeholder={t("commandPalette.searchPlaceholder")} />
        <CommandList>
          <CommandEmpty>{t("commandPalette.noResults")}</CommandEmpty>
          {COMMAND_GROUPS.map((group, i) => (
            <CommandGroup
              key={group.label ?? i}
              heading={group.label ? t(group.label) : t("commandPalette.defaultGroupHeading")}
            >
              {group.items.map((item) => (
                <CommandItem
                  key={item.href}
                  value={t(item.label)}
                  onSelect={() => go(item.href)}
                >
                  <item.icon className="size-4" />
                  {t(item.label)}
                </CommandItem>
              ))}
            </CommandGroup>
          ))}
        </CommandList>
      </Command>
    </CommandDialog>
  );
}
