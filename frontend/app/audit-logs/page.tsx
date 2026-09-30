import type { Metadata } from "next";

import { SectionPlaceholder } from "@/components/section-placeholder";

export const metadata: Metadata = { title: "Audit Logs" };

export default function Page() {
  return <SectionPlaceholder href="/audit-logs" />;
}
