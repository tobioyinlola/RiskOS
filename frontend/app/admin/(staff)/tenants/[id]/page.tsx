import type { Metadata } from "next";

import { TenantDetailPage } from "@/features/admin/tenants";

export const metadata: Metadata = { title: "Staff: tenant" };

export default async function Page({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <TenantDetailPage id={id} />;
}
