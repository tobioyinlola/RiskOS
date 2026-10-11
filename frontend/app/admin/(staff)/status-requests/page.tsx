import type { Metadata } from "next";

import { TenantStatusRequestsPage } from "@/features/admin/tenant-status";

export const metadata: Metadata = { title: "Staff: tenant status requests" };

export default function Page() {
  return <TenantStatusRequestsPage />;
}
