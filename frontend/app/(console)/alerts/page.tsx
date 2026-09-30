import type { Metadata } from "next";
import { Suspense } from "react";

import { AlertQueue } from "@/features/alerts/alerts";

export const metadata: Metadata = { title: "Alerts" };

export default function Page() {
  return (
    <Suspense>
      <AlertQueue />
    </Suspense>
  );
}
