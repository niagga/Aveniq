import "server-only";

import { headers } from "next/headers";

/**
 * The host name the browser used to open this page, without a port.
 *
 * In production the edge sets `x-forwarded-host`; in local development the
 * Host header is enough. A practice landing is keyed by this value: demo.localhost
 * is not the same screen as localhost.
 */
export async function requestHost(): Promise<string> {
  const headerList = await headers();
  const raw =
    headerList.get("x-forwarded-host")?.split(",")[0]?.trim() ??
    headerList.get("host") ??
    "";
  const withoutPort = raw.includes(":") ? raw.slice(0, raw.indexOf(":")) : raw;
  return withoutPort.toLowerCase();
}