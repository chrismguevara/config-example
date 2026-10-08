import { useQuery } from "@tanstack/react-query";
import { fetchSchema } from "../api/client";
import { ENUMS } from "../generated/user-settings";

type EnumName = keyof typeof ENUMS;

/**
 * Enum option lists for the UI.
 *
 * Compile-time: the generated ENUMS constant gives typed defaults.
 * Run-time: the backend's live schema overrides them, so a backend that added
 * an enum value (say a new map layer) shows it before the frontend is rebuilt.
 * The reverse (frontend ahead of backend) is harmless: the server rejects the
 * value with a 422 that the UI surfaces.
 */
export function useEnums() {
  const q = useQuery({ queryKey: ["settings-schema"], queryFn: fetchSchema, staleTime: 5 * 60_000 });
  const live = q.data?.$defs;
  const values = <N extends EnumName>(name: N): readonly (typeof ENUMS)[N][number][] => {
    const fromServer = live?.[name]?.enum as (typeof ENUMS)[N][number][] | undefined;
    return fromServer ?? ENUMS[name];
  };
  return { values, serverVersion: q.data?.properties?.schemaVersion?.const, isLoading: q.isLoading };
}
