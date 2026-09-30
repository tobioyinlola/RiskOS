import type { Metadata } from "next";
import { Suspense } from "react";

import { EventExplorer } from "@/features/events/events";

export const metadata: Metadata = { title: "Events" };

export default function Page() {
  return (
    <Suspense>
      <EventExplorer />
    </Suspense>
  );
}
