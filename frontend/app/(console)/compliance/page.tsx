import { redirect } from "next/navigation";

/** The rules library moved into Rules → Preset rules. */
export default function Page() {
  redirect("/rules?tab=presets");
}
