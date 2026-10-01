import type { Metadata } from "next";
import { IBM_Plex_Mono, IBM_Plex_Sans } from "next/font/google";
import { headers } from "next/headers";
import { connection } from "next/server";

import { THEME_SCRIPT } from "@/lib/preferences";

import "./globals.css";
import { Providers } from "./providers";

const plexSans = IBM_Plex_Sans({
  variable: "--font-plex-sans",
  subsets: ["latin"],
  weight: ["400", "500", "600"],
});

const plexMono = IBM_Plex_Mono({
  variable: "--font-plex-mono",
  subsets: ["latin"],
  weight: ["400", "500"],
});

export const metadata: Metadata = {
  title: {
    default: "RiskOS",
    template: "%s · RiskOS",
  },
  description: "Explainable risk decisions for financial events.",
  robots: { index: false, follow: false },
};

/**
 * The console defaults to the dark theme (Console); users can switch to light (Daylight). The
 * stored choice is applied before first paint by an inline script carrying this request's CSP
 * nonce, so the page never flashes the wrong theme. Every page renders per request for that
 * nonce (see proxy.ts).
 */
export default async function RootLayout({ children }: LayoutProps<"/">) {
  await connection();
  const nonce = (await headers()).get("x-nonce") ?? undefined;
  return (
    <html
      lang="en"
      data-theme="dark"
      suppressHydrationWarning
      className={`${plexSans.variable} ${plexMono.variable} h-full antialiased`}
    >
      <head>
        <script
          nonce={nonce}
          dangerouslySetInnerHTML={{ __html: THEME_SCRIPT }}
        />
      </head>
      <body className="min-h-full">
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
