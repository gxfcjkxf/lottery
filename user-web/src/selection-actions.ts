import type { RuleDefinition, RulePool, RuleTicketSelection } from '../../shared/src/rules';

export function emptyBetSelection(): RuleTicketSelection {
  return { regular: [], special: [], digits: [], exclude: [], attributes: {}, features: {} };
}

function poolValues(pool: RulePool): number[] {
  if (pool.values?.length) return [...new Set(pool.values)];
  if (pool.min === undefined || pool.max === undefined) return [];
  return Array.from({ length: pool.max - pool.min + 1 }, (_, index) => pool.min! + index);
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
      const regularPool = poolValues(model.regular_pool);
      result.regular = sample(regularPool, model.regular_pool.allow_repeat ? Math.min(selection.regular_count, regularPool.length) : selection.regular_count);
      const specialPool = poolValues(model.special_pool).filter(value => model.model !== 'M_SELECT_N' || !result.regular!.includes(value));
      result.special = sample(specialPool, model.special_pool.allow_repeat ? Math.min(selection.special_count, specialPool.length) : selection.special_count);
      if (!model.ordered) {
        result.regular.sort((a, b) => a - b);
        result.special.sort((a, b) => a - b);
      }
      return result;
    }
    case 'exclude': {
      const values = model.model === 'DIGITS_0_9' ? digits : [...new Set([...poolValues(model.regular_pool), ...poolValues(model.special_pool)])];
      result.exclude = sample(values, selection.exclude_count).sort((a, b) => a - b);
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
