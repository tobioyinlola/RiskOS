import type { Metadata } from "next";

import { RiskSettings } from "@/features/settings/settings";

export const metadata: Metadata = { title: "Risk settings" };

export default function Page() {
  return <RiskSettings />;
}
