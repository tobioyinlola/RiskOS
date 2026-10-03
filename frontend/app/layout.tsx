import type { Metadata } from "next";
import { connection } from "next/server";

import { brand, code } from "./fonts";
import "./globals.css";
import { Providers } from "./providers";

export const metadata: Metadata = {
  title: {
    default: "RiskOS",
    template: "%s · RiskOS",
  },
  description: "Explainable risk decisions for financial events.",
  robots: { index: false, follow: false },
};

/**
 * Every page renders per request so Next.js can stamp this request's CSP nonce on its scripts
 * (see proxy.ts). The product is light only (docs/design-system.md).
 */
export default async function RootLayout({ children }: LayoutProps<"/">) {
  await connection();
  return (
    <html
      lang="en"
      className={`${brand.variable} ${code.variable} h-full antialiased`}
    >
      <body className="min-h-full">
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
