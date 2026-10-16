import { useQuery } from "@tanstack/react-query";
import { api, type PublicReferenceProfile } from "@/api";

export function usePublicReferences(enabled: boolean) {
  return useQuery({
    queryKey: ["model-check-public-references"],
    queryFn: async ({ signal }) => {
      const data = await api.modelCheckCapabilities(signal);
      if (!data.public_references) throw new Error("后端尚未提供公开指纹目录，请更新后重试");
      return data.public_references;
    },
    enabled,
    staleTime: 60_000,
  });
}

export function publicReferenceProfile(
  model: string,
  profiles: PublicReferenceProfile[] | undefined,
): PublicReferenceProfile | undefined {
  return profiles?.find((profile) => profile.aliases.includes(model.trim()));
}

export function publicReferenceBudget(
  models: string[],
  profiles: PublicReferenceProfile[] | undefined,
): number {
  return models.reduce(
    (sum, model) => sum + (publicReferenceProfile(model, profiles)?.requests ?? 0),
    0,
  );
}
