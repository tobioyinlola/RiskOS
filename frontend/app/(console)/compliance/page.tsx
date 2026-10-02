import type { Metadata } from "next";

import { ComplianceLibrary } from "@/features/compliance/library";

export const metadata: Metadata = { title: "Rules library" };

export default function Page() {
  return <ComplianceLibrary />;
}
