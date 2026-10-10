"use client";

import { useCallback } from "react";
import { useTranslation } from "react-i18next";

import { ApiError, TRANSLATED_ERROR_CODES } from "@/lib/api/client";

/** Turns a query error into what an ErrorState shows: the translated text
 * for a known API error code, otherwise the error's own message. */
export function useApiErrorMessage() {
  const { t } = useTranslation("common");
  return useCallback(
    (error: Error) =>
      error instanceof ApiError && TRANSLATED_ERROR_CODES.has(error.code) ? t(`apiErrors.${error.code}`) : error.message,
    [t],
  );
}
