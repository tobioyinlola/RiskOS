import type { Metadata } from "next";

import { SectionPlaceholder } from "@/components/section-placeholder";

export const metadata: Metadata = { title: "Events" };

export default function Page() {
  return <SectionPlaceholder href="/events" />;
}
