import type { Metadata } from "next";

import { Overview } from "@/features/overview/overview";

// The root layout's title template does not apply to its own segment, so the title is absolute.
export const metadata: Metadata = { title: { absolute: "Overview · RiskOS" } };

export default function Page() {
  return <Overview />;
}
