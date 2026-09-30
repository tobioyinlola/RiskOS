import type { Metadata } from "next";

import { NewRule } from "@/features/rules/rules";

export const metadata: Metadata = { title: "Create rule" };

export default function Page() {
  return <NewRule />;
}
