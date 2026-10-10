import type { Metadata } from "next";

import { TenantsPage } from "@/features/admin/tenants";

export const metadata: Metadata = { title: "Staff: tenants" };

export default function Page() {
  return <TenantsPage />;
}
