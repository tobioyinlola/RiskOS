/**
 * Same-origin proxy to the RiskOS internal admin API (/api/admin/v1/*), exactly like the customer
 * API proxy: the staff session cookie stays HttpOnly and SameSite=Strict.
 */
import { GET as proxy } from "../../../v1/[...path]/route";

export const GET = proxy;
export const POST = proxy;
export const PUT = proxy;
export const PATCH = proxy;
export const DELETE = proxy;
export const dynamic = "force-dynamic";
