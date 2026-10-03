import type { Metadata } from "next";

import { Landing } from "@/features/marketing/landing";

// The marketing site is public and indexable, unlike the console.
export const metadata: Metadata = {
  title: {
    absolute: "RiskOS · Explainable risk decisions for financial events",
  },
  description:
    "RiskOS scores every payment, login and payout as it happens, explains the score and routes the risky ones to your analysts.",
  robots: { index: true, follow: true },
};

export default function Page() {
  return <Landing />;
}
