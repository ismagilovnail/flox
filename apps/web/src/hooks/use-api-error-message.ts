"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";

import { ApiError } from "@/lib/api/client";

/** API error codes with a translated, actionable message
 * (common:apiErrors.*). Any other error shows the server's message. */
const TRANSLATED_CODES = new Set(["query_too_heavy"]);

/** Turns a query error into what an ErrorState shows: the translated text
 * for a known API error code, otherwise the error's own message. */
export function useApiErrorMessage() {
  const { t } = useTranslation("common");
  return useCallback(
    (error: Error) =>
      error instanceof ApiError && TRANSLATED_CODES.has(error.code) ? t(`apiErrors.${error.code}`) : error.message,
    [t],
  );
}
