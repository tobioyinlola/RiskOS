import type { Metadata } from "next";

import { LiveMonitor } from "@/features/live/live";

export const metadata: Metadata = { title: "Live monitor" };

export default function Page() {
  return <LiveMonitor />;
}
