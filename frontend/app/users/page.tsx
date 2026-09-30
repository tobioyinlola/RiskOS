import type { Metadata } from "next";

import { SectionPlaceholder } from "@/components/section-placeholder";

export const metadata: Metadata = { title: "Users" };

export default function Page() {
  return <SectionPlaceholder href="/users" />;
}
