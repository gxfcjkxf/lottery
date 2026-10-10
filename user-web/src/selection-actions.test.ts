import { afterEach, describe, expect, it, vi } from 'vitest';
import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import BetSelection from './BetSelection.vue';
import type { RuleDefinition } from '../../shared/src/rules';
import { emptyBetSelection, randomBetSelection } from './selection-actions';

const definition = (): RuleDefinition => ({
  schema_version: 1,
  model: { model: 'X_PLUS_Y', regular_pool: { min: 1, max: 6, allow_repeat: false }, special_pool: { values: [7, 19, 31, 43], allow_repeat: false }, regular_count: 3, special_count: 1, pool_size: 0, total_count: 0, length: 0, allow_repeat: false, ordered: false },
  selection: { mode: 'numbers', regular_count: 3, special_count: 1, exclude_count: 0, attribute_groups: null, feature_choices: null },
  number_attributes: null, unit_points: '1', prize_tiers: [], mixed_tier_policy: 'sum', cap_points: null, rounding: 'half_up', rounding_scope: 'ticket', limits: { max_combinations: 100, max_multiplier: '10', max_bet_points: null },
});
afterEach(() => vi.restoreAllMocks());

describe('editable selection actions', () => {
  it('renders both controls in English and Chinese without submitting a form', async () => {
    for (const [locale, clear, random] of [['en', 'Clear selection', 'Random selection'], ['zh', '清空选号', '随机选号']] as const) {
      const html = await renderToString(createSSRApp(BetSelection, { definition: definition(), modelValue: emptyBetSelection(), locale }));
      expect(html).toContain(clear); expect(html).toContain(random);
      expect(html.match(/<button type="button"/g)).toHaveLength(2);
    }
  });
  it('disables both controls when the selection is locked', async () => {
    const html = await renderToString(createSSRApp(BetSelection, { definition: definition(), modelValue: emptyBetSelection(), locale: 'en', disabled: true }));
    expect(html.match(/<button type="button" disabled/g)).toHaveLength(2);
  });
  it('shows all configured attributes beside regular and special numbers without inventing labels', async () => {
    const rule = definition();
    rule.number_attributes = { color: { red: [1, 7] }, parity: { odd: [1, 3, 7] } };
    const html = await renderToString(createSSRApp(BetSelection, { definition: rule, modelValue: emptyBetSelection(), locale: 'en' }));
    const labels = html.split('</label>');
    for (const choice of ['Choose regular number 1', 'Choose special number 7']) {
      const label = labels.find(value => value.includes(`aria-label="${choice}"`));
      expect(label).toContain('color: red'); expect(label).toContain('parity: odd');
    }
    expect(labels.find(value => value.includes('aria-label="Choose regular number 2"'))).not.toContain('number-attributes');
  });
  it('clears every selection field using independent arrays and maps', () => {
    const first = emptyBetSelection();
    first.regular!.push(1); first.attributes!.color = ['red'];
    expect(emptyBetSelection()).toEqual({ regular: [], special: [], digits: [], exclude: [], attributes: {}, features: {} });
  });
  it('uses the configured X+Y pools and counts without changing the definition', () => {
    vi.spyOn(Math, 'random').mockReturnValue(0);
    const rule = definition(), before = structuredClone(rule);
    expect(randomBetSelection(rule)).toEqual({ ...emptyBetSelection(), regular: [1, 2, 3], special: [7] });
    expect(rule).toEqual(before);
  });
  it('keeps M-select-N regular and special numbers distinct across one pool', () => {
    vi.spyOn(Math, 'random').mockReturnValue(0);
    const rule = definition();
    rule.model.model = 'M_SELECT_N'; rule.model.special_pool = { ...rule.model.regular_pool };
    rule.model.pool_size = 6; rule.model.total_count = 4;
    expect(randomBetSelection(rule)).toMatchObject({ regular: [1, 2, 3], special: [4] });
  });
  it('respects whether digit positions may repeat, including zero', () => {
    vi.spyOn(Math, 'random').mockReturnValue(0);
    const rule = definition();
    rule.model = { ...rule.model, model: 'DIGITS_0_9', length: 3, regular_count: 0, special_count: 0, regular_pool: { allow_repeat: false }, special_pool: { allow_repeat: false }, ordered: true };
    rule.selection.regular_count = 0; rule.selection.special_count = 0;
    expect(randomBetSelection(rule).digits).toEqual([[0], [1], [2]]);
    rule.model.allow_repeat = true;
    expect(randomBetSelection(rule).digits).toEqual([[0], [0], [0]]);
  });
  it('randomizes exclusion, attributes and feature choices rather than raw numbers', () => {
    vi.spyOn(Math, 'random').mockReturnValue(0);
    const rule = definition();
    rule.selection.mode = 'exclude'; rule.selection.exclude_count = 3;
    rule.selection.regular_count = 0; rule.selection.special_count = 0;
    expect(randomBetSelection(rule)).toEqual({ ...emptyBetSelection(), exclude: [1, 2, 3] });
    rule.selection.mode = 'attributes'; rule.selection.exclude_count = 0; rule.selection.attribute_groups = ['color', 'parity'];
    rule.number_attributes = { color: { red: [1, 3], blue: [2, 4] }, parity: { odd: [1, 3], even: [2, 4] } };
    expect(randomBetSelection(rule)).toEqual({ ...emptyBetSelection(), attributes: { color: ['red'], parity: ['odd'] } });
    rule.selection.mode = 'features'; rule.selection.attribute_groups = null; rule.selection.feature_choices = { odd_count: [0, 1, 2, 3], all_same: [0, 1] };
    expect(randomBetSelection(rule)).toEqual({ ...emptyBetSelection(), features: { odd_count: [0], all_same: [0] } });
  });
  it('supports a repeat-enabled pool smaller than the required drawn count', () => {
    const rule = definition();
    rule.model.regular_pool = { values: [7], allow_repeat: true };
    expect(randomBetSelection(rule).regular).toEqual([7]);
  });
  it('does not silently replace unsupported modes or impossible distinct choices', () => {
    const rule = definition(); rule.selection.mode = 'script';
    expect(() => randomBetSelection(rule)).toThrow('Unsupported selection mode');
    rule.selection.mode = 'numbers'; rule.selection.regular_count = 7;
    expect(() => randomBetSelection(rule)).toThrow('exceeds its number pool');
  });
});
