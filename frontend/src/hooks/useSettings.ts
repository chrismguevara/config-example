import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError, getUser, type SettingsResponse } from "../api/client";

const key = () => ["settings", getUser()];

/** The user's current settings document (defaults if they never saved any). */
export function useSettings() {
  return useQuery({ queryKey: key(), queryFn: api.get });
}

/**
 * Wrap any api.* call: on success the whole document from the server replaces
 * the cache (the server is the source of truth, including after a lazy schema
 * upgrade); on a 412 conflict the cache is refetched so the user sees what
 * changed under them.
 */
export function useSettingsMutation<A extends unknown[]>(fn: (...args: A) => Promise<SettingsResponse>) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (args: A) => fn(...args),
    onSuccess: (data) => qc.setQueryData(key(), data),
    onError: (err) => {
      if (err instanceof ApiError && err.isConflict) void qc.invalidateQueries({ queryKey: key() });
    },
  });
}
