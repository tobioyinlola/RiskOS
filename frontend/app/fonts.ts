import { IBM_Plex_Mono, Inter } from "next/font/google";

/**
 * The brand typeface is Inter. Every component reads it through --font-sans (app/globals.css),
 * so changing the face is a change here only.
 */
export const brand = Inter({
  variable: "--font-brand",
  subsets: ["latin"],
  weight: ["400", "500", "600", "700"],
  style: ["normal", "italic"],
});

/** IDs, keys, hashes and payloads. */
export const code = IBM_Plex_Mono({
  variable: "--font-code",
  subsets: ["latin"],
  weight: ["400", "500"],
});
