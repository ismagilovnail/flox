import type { Metadata } from "next";
import { cookies, headers } from "next/headers";
import { Geist, Geist_Mono } from "next/font/google";
import { ThemeProvider } from "@/components/theme-provider";
import { QueryProvider } from "@/components/query-provider";
import { I18nProvider } from "@/components/i18n-provider";
import { TooltipProvider } from "@/components/ui/tooltip";
import { Toaster } from "@/components/ui/sonner";
import { LOCALE_COOKIE, resolveLocale } from "@/lib/i18n/locale";
import "./globals.css";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "FLOX — Track. Route. Optimize.",
  description:
    "FLOX is a production-grade SaaS traffic tracking and routing platform.",
};

// Reading cookies()/headers() here opts every route into per-request
// dynamic rendering (Next.js can no longer statically prerender any
// page — see the "Good to know" notes on next/headers' cookies/headers
// docs). A deliberate trade-off for this app: the alternative is a
// client-only "flash to the real value after mount" step that races a
// Suspense-deferred hydration commit and cannot be made fully reliable
// (see components/i18n-provider.tsx). Every page already fetches its
// real data client-side via TanStack Query, so static prerendering was
// only ever serving an empty app shell — losing it costs little here.
export default async function RootLayout({ children }: LayoutProps<"/">) {
  const [cookieStore, headerList] = await Promise.all([cookies(), headers()]);
  const locale = resolveLocale(cookieStore.get(LOCALE_COOKIE)?.value, headerList.get("accept-language"));

  return (
    <html
      lang={locale}
      className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}
      suppressHydrationWarning
    >
      <body className="min-h-full flex flex-col">
        <ThemeProvider
          attribute="class"
          defaultTheme="dark"
          enableSystem={false}
          disableTransitionOnChange
        >
          <I18nProvider initialLocale={locale}>
            <QueryProvider>
              <TooltipProvider>{children}</TooltipProvider>
              <Toaster />
            </QueryProvider>
          </I18nProvider>
        </ThemeProvider>
      </body>
    </html>
  );
}
