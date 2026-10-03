import type { Metadata } from "next";

import { WorkQueue } from "@/features/compliance/work";

export const metadata: Metadata = { title: "Work queue" };

export default function Page() {
  return <WorkQueue />;
}
