"use client";

import { useTheme } from "next-themes";
import { MoonIcon, SunIcon } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useSyncExternalStore } from "react";

import { IconButton } from "@/components/ui/icon-button";

const emptySubscribe = () => () => {};

/** Avoids an SSR/client icon mismatch without setState-in-effect. */
function useMounted() {
  return useSyncExternalStore(
    emptySubscribe,
    () => true,
    () => false,
  );
}

export function ThemeToggle() {
  const { t } = useTranslation("common");
  const { resolvedTheme, setTheme } = useTheme();
  const mounted = useMounted();

  if (!mounted) {
    return <IconButton aria-label={t("a11y.toggleTheme")} disabled />;
  }

  const isDark = resolvedTheme === "dark";

  return (
    <IconButton
      aria-label={isDark ? t("a11y.lightTheme") : t("a11y.darkTheme")}
      onClick={() => setTheme(isDark ? "light" : "dark")}
    >
      {isDark ? (
        <SunIcon className="size-4" />
      ) : (
        <MoonIcon className="size-4" />
      )}
    </IconButton>
  );
}
