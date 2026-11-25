"use client";

import { useCallback, useEffect, useState } from "react";

import type { Client } from "@/entities/client/model";
import { browserApi } from "@/shared/api/client";

/**
 * The practice's billing clients, deactivated ones included: a receipt already issued names
 * a client that may have stopped being billed, and the list has to keep resolving it.
 */
export function useClients(enabled: boolean) {
  const [clients, setClients] = useState<Client[] | null>(null);
  const [failedStatus, setFailedStatus] = useState<number | null>(null);

  const load = useCallback(async () => {
    const { data, response } = await browserApi.GET("/api/v1/clients", {
      params: { query: {} },
    });
    if (!response.ok) {
      setFailedStatus(response.status);
      return;
    }
    setFailedStatus(null);
    setClients(data?.items ?? []);
  }, []);

  useEffect(() => {
    if (!enabled) return;
    let active = true;
    void (async () => {
      if (active) await load();
    })();
    return () => {
      active = false;
    };
  }, [enabled, load]);

  return { clients, failedStatus, reload: load };
}

/**
 * One client by id, for the screens that read a patient and have to show who pays. The
 * request is skipped while the id is empty, so a patient without a client loads nothing.
 */
export function useClient(id: string | undefined) {
  const [client, setClient] = useState<Client | null>(null);
  const [failedStatus, setFailedStatus] = useState<number | null>(null);

  useEffect(() => {
    if (!id) return;
    let active = true;
    void (async () => {
      const { data, response } = await browserApi.GET("/api/v1/clients/{id}", {
        params: { path: { id } },
      });
      if (!active) return;
      if (!response.ok) {
        setFailedStatus(response.status);
        return;
      }
      setFailedStatus(null);
      setClient(data ?? null);
    })();
    return () => {
      active = false;
    };
  }, [id]);

  return { client, failedStatus };
}
