import type { Metadata } from "next";
import { notFound } from "next/navigation";

import { EntityProfile } from "@/features/entities/entities";
import type { EntityKind } from "@/lib/queries";

export const metadata: Metadata = { title: "Entity" };

const KINDS: EntityKind[] = ["user", "account", "device"];

export default async function Page({
  params,
}: PageProps<"/entities/[kind]/[ref]">) {
  const { kind, ref } = await params;
  if (!KINDS.includes(kind as EntityKind)) notFound();
  return (
    <EntityProfile kind={kind as EntityKind} refId={decodeURIComponent(ref)} />
  );
}
