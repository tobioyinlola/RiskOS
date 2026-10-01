import type { Metadata } from "next";

import { EntityLookup } from "@/features/entities/entities";

export const metadata: Metadata = { title: "Entities" };

export default function Page() {
  return <EntityLookup />;
}
