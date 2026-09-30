import type { Metadata } from "next";

import { RuleDetail } from "@/features/rules/rules";

export const metadata: Metadata = { title: "Rule" };

export default async function Page({ params }: PageProps<"/rules/[ruleId]">) {
  const { ruleId } = await params;
  return <RuleDetail id={ruleId} />;
}
