import type { Metadata } from "next";

import { SecurityOverview } from "@/features/admin/pages";

export const metadata: Metadata = { title: "Staff: security" };

export default function Page() {
  return <SecurityOverview />;
}
