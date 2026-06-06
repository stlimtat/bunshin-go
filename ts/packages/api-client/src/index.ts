import createClient from "openapi-fetch";
import type { paths } from "./schema.d.ts";

export type { paths };

/**
 * Creates a typed bunshin API client.
 * @param baseUrl - Base URL of the bunshin server (e.g. "http://localhost:8080")
 */
export function createBunshinClient(baseUrl: string) {
  return createClient<paths>({ baseUrl });
}

export type BunshinClient = ReturnType<typeof createBunshinClient>;
