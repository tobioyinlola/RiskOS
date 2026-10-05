import type { Metadata } from "next";
import type { ReactNode } from "react";

import { DocsShell } from "@/features/docs/shell";

// The developer documentation is public and indexable, like the rest of the marketing site.
export const metadata: Metadata = {
  title: {
    template: "%s · RiskOS developer docs",
    default: "RiskOS developer docs",
  },
  description:
    "Integrate RiskOS end to end: send events, act on risk decisions, receive webhooks, configure rules and rule packs, and the complete API reference.",
  robots: { index: true, follow: true },
};

export default function Layout({ children }: { children: ReactNode }) {
  return <DocsShell>{children}</DocsShell>;
}
