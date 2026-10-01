import type { Metadata } from "next";

import { SupportAccessPage } from "@/features/admin/pages";

export const metadata: Metadata = { title: "Staff: support access" };

export default function Page() {
  return <SupportAccessPage />;
}
