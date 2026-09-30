import type { Metadata } from "next";

import { Team } from "@/features/org/org";

export const metadata: Metadata = { title: "Team" };

export default function Page() {
  return <Team />;
}
