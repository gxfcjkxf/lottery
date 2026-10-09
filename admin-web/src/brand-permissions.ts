export interface BrandPermissionAccount {
  super_admin: boolean;
  brand_ids: string[];
  permissions_by_brand?: Record<string, string[]>;
}

// Brand staff grants are scoped by the server's current brand mapping. Neither
// flat permission arrays nor platform grants can substitute for that mapping.
export function brandPermissionSet(account: BrandPermissionAccount, brandId: string): Set<string> {
  if (account.super_admin !== false || !account.brand_ids?.includes(brandId)) return new Set();
  return new Set(account.permissions_by_brand?.[brandId] ?? []);
}
