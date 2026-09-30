import type { Metadata } from "next";

import { SectionPlaceholder } from "@/components/section-placeholder";

// The root layout title template does not apply to its own segment, so the title is absolute.
export const metadata: Metadata = { title: { absolute: "Overview · RiskOS" } };

export default function Page() {
  return <SectionPlaceholder href="/" />;
}
