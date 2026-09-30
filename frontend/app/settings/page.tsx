import type { Metadata } from "next";

import { SectionPlaceholder } from "@/components/section-placeholder";

export const metadata: Metadata = { title: "Settings" };

export default function Page() {
  return <SectionPlaceholder href="/settings" />;
}
