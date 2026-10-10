import type { RuleDefinition, RuleModel, RulePool, RuleTicketSelection } from '../../shared/src/rules';

export function emptyBetSelection(): RuleTicketSelection {
  return { regular: [], special: [], digits: [], exclude: [], attributes: {}, features: {} };
}

export function rulePoolValues(pool: RulePool): number[] {
  if (pool.values?.length) return [...new Set(pool.values)].sort((a, b) => a - b);
  // The current Go DTO omits zero bounds; omission represents zero.
  const min = pool.min === undefined ? 0 : pool.min;
  const max = pool.max === undefined ? 0 : pool.max;
  if (!Number.isInteger(min) || !Number.isInteger(max) || max < min || max - min > 9999) {
    throw new Error('Invalid configured number pool');
  }
  return Array.from({ length: max - min + 1 }, (_, index) => min + index);
}

export function exclusionPoolValues(model: RuleModel): number[] {
  if (model.model === 'DIGITS_0_9') return Array.from({ length: 10 }, (_, index) => index);
  if (model.model !== 'X_PLUS_Y' && model.model !== 'M_SELECT_N') throw new Error('Unsupported number model');
  const values = [{ pool: model.regular_pool, count: model.regular_count }, { pool: model.special_pool, count: model.special_count }]
    .filter(({ pool, count }) => count > 0 || pool.values?.length || pool.min !== undefined || pool.max !== undefined)
    .flatMap(({ pool }) => rulePoolValues(pool));
  return [...new Set(values)].sort((a, b) => a - b);
}

function sample<T>(values: T[], count: number): T[] {
  if (count > values.length) throw new Error('The configured selection exceeds its number pool');
  const candidates = [...values];
  for (let index = 0; index < count; index++) {
    const selected = index + Math.floor(Math.random() * (candidates.length - index));
    [candidates[index], candidates[selected]] = [candidates[selected], candidates[index]];
  }
  return candidates.slice(0, count);
}

// This only suggests an editable ticket. Draws and quotes remain server-side.
export function randomBetSelection(definition: RuleDefinition): RuleTicketSelection {
  const result = emptyBetSelection();
  const { model, selection } = definition;
  const digits = Array.from({ length: 10 }, (_, index) => index);
  switch (selection.mode) {
    case 'numbers': {
      if (model.model === 'DIGITS_0_9') {
        result.digits = model.allow_repeat
          ? Array.from({ length: model.length }, () => sample(digits, 1))
          : sample(digits, model.length).map(value => [value]);
        return result;
      }
      if (model.model !== 'X_PLUS_Y' && model.model !== 'M_SELECT_N') throw new Error('Unsupported number model');
      const regularPool = rulePoolValues(model.regular_pool);
      result.regular = sample(regularPool, model.regular_pool.allow_repeat ? Math.min(selection.regular_count, regularPool.length) : selection.regular_count);
      const specialPool = rulePoolValues(model.special_pool).filter(value => model.model !== 'M_SELECT_N' || !result.regular!.includes(value));
      result.special = sample(specialPool, model.special_pool.allow_repeat ? Math.min(selection.special_count, specialPool.length) : selection.special_count);
      if (!model.ordered) {
        result.regular.sort((a, b) => a - b);
        result.special.sort((a, b) => a - b);
      }
      return result;
    }
    case 'exclude': {
      result.exclude = sample(exclusionPoolValues(model), selection.exclude_count).sort((a, b) => a - b);
      return result;
    }
    case 'attributes':
      for (const group of selection.attribute_groups ?? []) {
        const values = Object.keys(definition.number_attributes?.[group] ?? {});
        if (!values.length) throw new Error('The configured attribute group has no choices');
        result.attributes![group] = sample(values, 1);
      }
      return result;
    case 'features':
      for (const [name, values] of Object.entries(selection.feature_choices ?? {})) {
        result.features![name] = sample(values, 1);
      }
      return result;
    default:
      throw new Error('Unsupported selection mode');
  }
}
