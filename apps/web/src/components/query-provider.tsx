"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";

export function QueryProvider({ children }: { children: React.ReactNode }) {
  // Created once per component instance (not module scope) — a module-level
  // singleton would leak cached data across users on the server; useState's
  // lazy initializer keeps this stable across client re-renders instead.
  const [client] = useState(() => new QueryClient());
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}
