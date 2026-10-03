import type { Metadata } from "next";

import { ComplianceRule } from "@/features/compliance/rule";

export const metadata: Metadata = { title: "Compliance rule" };

export default async function Page({
  params,
}: PageProps<"/compliance/rules/[ruleId]">) {
  const { ruleId } = await params;
  return <ComplianceRule id={ruleId} />;
}
