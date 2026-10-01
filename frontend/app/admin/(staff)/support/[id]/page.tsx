import type { Metadata } from "next";

import { SupportView } from "@/features/admin/support-view";

export const metadata: Metadata = { title: "Staff: read-only view" };

export default async function Page({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <SupportView id={id} />;
}
