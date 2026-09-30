import type { Metadata } from "next";

import { CaseWorkspace } from "@/features/cases/cases";

export const metadata: Metadata = { title: "Case" };

export default async function Page({ params }: PageProps<"/cases/[caseId]">) {
  const { caseId } = await params;
  return <CaseWorkspace id={caseId} />;
}
