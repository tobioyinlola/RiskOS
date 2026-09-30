import type { Metadata } from "next";

import { SectionPlaceholder } from "@/components/section-placeholder";

export const metadata: Metadata = { title: "Team" };

export default function Page() {
  return <SectionPlaceholder href="/team" />;
}
