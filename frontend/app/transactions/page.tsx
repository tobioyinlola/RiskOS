import type { Metadata } from "next";

import { SectionPlaceholder } from "@/components/section-placeholder";

export const metadata: Metadata = { title: "Transactions" };

export default function Page() {
  return <SectionPlaceholder href="/transactions" />;
}
