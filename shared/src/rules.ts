/** Schema-v1 rule and ticket-selection JSON shared with backend/internal/rules. */
export interface RulePool {
  /** Current Go JSON omits zero bounds; an omitted bound represents zero. */
  min?: number;
  max?: number;
  values?: number[] | null;
  allow_repeat: boolean;
}

export interface RuleModel {
  model: string;
  regular_pool: RulePool;
  special_pool: RulePool;
  regular_count: number;
  special_count: number;
  pool_size: number;
  total_count: number;
  length: number;
  allow_repeat: boolean;
  ordered: boolean;
}

export interface RuleSelectionDefinition {
  mode: string;
  regular_count: number;
  special_count: number;
  exclude_count: number;
  attribute_groups: string[] | null;
  feature_choices: Record<string, number[]> | null;
}

export interface RuleCondition {
  op: string;
  field?: string;
  target?: string;
  position?: number;
  attribute_group?: string;
  attribute_value?: string;
  selection_key?: string;
  value?: number;
  values?: number[] | null;
  min?: number;
  max?: number;
  children?: RuleCondition[] | null;
}

export interface RuleTier {
  code: string;
  condition: RuleCondition;
  odds: string;
  exclusive: boolean;
  cap_points: string | null;
}

export interface RuleDefinition {
  schema_version: number;
  model: RuleModel;
  selection: RuleSelectionDefinition;
  number_attributes: Record<string, Record<string, number[]>> | null;
  unit_points: string;
  prize_tiers: RuleTier[] | null;
  mixed_tier_policy: string;
  cap_points: string | null;
  rounding: string;
  rounding_scope: string;
  limits: {
    max_combinations: number;
    max_multiplier: string;
    max_bet_points: string | null;
  };
}

export interface RuleTicketSelection {
  regular: number[] | null;
  special: number[] | null;
  digits: number[][] | null;
  exclude: number[] | null;
  attributes: Record<string, string[]> | null;
  features: Record<string, number[]> | null;
}
