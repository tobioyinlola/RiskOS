import type { Metadata } from "next";

import { SectionPlaceholder } from "@/components/section-placeholder";

export const metadata: Metadata = { title: "Risk Monitor" };

export default function Page() {
  return <SectionPlaceholder href="/risk-monitor" />;
}
