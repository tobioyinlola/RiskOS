import type { Metadata } from "next";

import { CaseList } from "@/features/cases/cases";

export const metadata: Metadata = { title: "Cases" };

export default function Page() {
  return <CaseList />;
}
