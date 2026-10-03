import type { Metadata } from "next";
import { Suspense } from "react";

import { GoogleCallback } from "@/features/auth/signup";

export const metadata: Metadata = { title: "Continuing with Google" };

export default function Page() {
  return (
    <Suspense>
      <GoogleCallback />
    </Suspense>
  );
}
