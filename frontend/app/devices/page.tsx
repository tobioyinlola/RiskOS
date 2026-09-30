import type { Metadata } from "next";

import { SectionPlaceholder } from "@/components/section-placeholder";

export const metadata: Metadata = { title: "Devices" };

export default function Page() {
  return <SectionPlaceholder href="/devices" />;
}
