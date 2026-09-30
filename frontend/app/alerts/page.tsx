import type { Metadata } from "next";

import { SectionPlaceholder } from "@/components/section-placeholder";

export const metadata: Metadata = { title: "Alerts" };

export default function Page() {
  return <SectionPlaceholder href="/alerts" />;
}
