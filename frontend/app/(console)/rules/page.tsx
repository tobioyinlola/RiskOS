import type { Metadata } from "next";

import { RuleList } from "@/features/rules/rules";

export const metadata: Metadata = { title: "Rules" };

export default function Page() {
  return <RuleList />;
}
