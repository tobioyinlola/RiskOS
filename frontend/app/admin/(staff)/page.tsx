import type { Metadata } from "next";

import { AdminOverviewPage } from "@/features/admin/tenants";

export const metadata: Metadata = { title: "Staff: overview" };

export default function Page() {
  return <AdminOverviewPage />;
}
