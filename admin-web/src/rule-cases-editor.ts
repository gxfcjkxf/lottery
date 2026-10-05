import type {
  RuleDefinition,
  RuleDraw,
  RuleTicketSelection,
} from "./rule-simulation-api";
import type { RuleValidationCase } from "./rule-versions-api";

const MAX_INT64 = 9_223_372_036_854_775_807n;
const MAX_NUMBER_VALUE = 1_000_000;
const MAX_FEATURE_VALUE = 20_000_000;

type LooseArray<T> = T[] | null | undefined;

function fail(message: string): never {
  throw new Error(message);
}

function utf8Length(value: string): number {
  return new TextEncoder().encode(value).length;
}

function isValidUtf8String(value: string): boolean {
  const bytes = new TextEncoder().encode(value);
  return new TextDecoder("utf-8", { fatal: true }).decode(bytes) === value;
}

function canonicalInt64(value: unknown, field: string, minimum = 0n): bigint {
  if (typeof value !== "string" || !/^(0|[1-9][0-9]*)$/.test(value))
    fail(`${field} must be a canonical non-negative integer string.`);
  const parsed = BigInt(value);
  if (parsed < minimum || parsed > MAX_INT64)
    fail(`${field} must be between ${minimum} and ${MAX_INT64}.`);
  return parsed;
}

function asArray<T>(value: LooseArray<T>, field: string): T[] {
  if (value == null) return [];
  if (!Array.isArray(value)) fail(`${field} must be an array.`);
  return value;
}

function intValues(value: LooseArray<number>, field: string): number[] {
  const values = asArray(value, field);
  for (const item of values) {
    if (!Number.isSafeInteger(item) || item < 0 || item > MAX_FEATURE_VALUE)
      fail(`${field} contains an invalid integer.`);
  }
  return values;
}

function poolValues(definition: RuleDefinition, special: boolean): number[] {
  const pool = special
    ? definition.model.special_pool
    : definition.model.regular_pool;
  const explicit = asArray(
    pool?.values,
    special ? "special_pool.values" : "regular_pool.values",
  );
  if (explicit.length) return explicit;
  const min = pool?.min ?? 0;
  const max = pool?.max ?? -1;
  if (
    !Number.isInteger(min) ||
    !Number.isInteger(max) ||
    min < 0 ||
    max < min ||
    max - min >= 10_000
  )
    return [];
  return Array.from({ length: max - min + 1 }, (_, index) => min + index);
}

function unique(values: number[], field: string): number[] {
  const seen = new Set<number>();
  for (const value of values) {
    if (seen.has(value)) fail(`${field} cannot contain duplicate values.`);
    seen.add(value);
  }
  return values;
}

function assertCount(
  values: number[],
  count: number,
  field: string,
  max = 100,
) {
  if (values.length < 1 || values.length > max || values.length < count)
    fail(`${field} must contain enough candidates for its configured count.`);
}

function drawPool(
  values: number[],
  count: number,
  allowRepeat: boolean,
  field: string,
) {
  if (
    !Number.isInteger(count) ||
    count < 0 ||
    (values.length === 0 && count > 0)
  )
    fail(`${field} cannot satisfy the draw count.`);
  if (!allowRepeat && count > values.length)
    fail(`${field} cannot repeat values and its pool is too small.`);
}

function assertDrawValues(
  values: number[],
  count: number,
  allowed: number[],
  allowRepeat: boolean,
  field: string,
) {
  if (values.length !== count)
    fail(`${field} must contain exactly ${count} values.`);
  const allowedSet = new Set(allowed);
  for (const value of values)
    if (!allowedSet.has(value))
      fail(`${field} contains a value outside its pool.`);
  if (!allowRepeat) unique(values, field);
}

function distinctAssignmentExists(positions: number[][]): boolean {
  const owner = Array<number>(10).fill(-1);
  const visit = (position: number, seen: boolean[]) => {
    for (const digit of positions[position]!) {
      if (seen[digit]) continue;
      seen[digit] = true;
      if (owner[digit] === -1 || visit(owner[digit]!, seen)) {
        owner[digit] = position;
        return true;
      }
    }
    return false;
  };
  return positions.every((_, index) =>
    visit(index, Array<boolean>(10).fill(false)),
  );
}

function selectionOf(input: RuleTicketSelection): RuleTicketSelection {
  const selection = input as RuleTicketSelection & Record<string, unknown>;
  const attributes = selection.attributes ?? {};
  const features = selection.features ?? {};
  if (typeof attributes !== "object" || Array.isArray(attributes))
    fail("selection.attributes must be an object.");
  if (typeof features !== "object" || Array.isArray(features))
    fail("selection.features must be an object.");
  const digits = asArray(
    selection.digits as LooseArray<number[]>,
    "selection.digits",
  ).map((values, index) =>
    intValues(values as LooseArray<number>, `selection.digits[${index}]`),
  );
  const selectedAttributes: Record<string, string[]> = {};
  for (const [group, values] of Object.entries(attributes))
    selectedAttributes[group] = asArray(
      values as LooseArray<string>,
      `selection.attributes.${group}`,
    );
  const selectedFeatures: Record<string, number[]> = {};
  for (const [key, values] of Object.entries(features))
    selectedFeatures[key] = intValues(
      values as LooseArray<number>,
      `selection.features.${key}`,
    );
  return {
    regular: intValues(
      selection.regular as LooseArray<number>,
      "selection.regular",
    ),
    special: intValues(
      selection.special as LooseArray<number>,
      "selection.special",
    ),
    digits,
    exclude: intValues(
      selection.exclude as LooseArray<number>,
      "selection.exclude",
    ),
    attributes: selectedAttributes,
    features: selectedFeatures,
  };
}

function drawOf(input: RuleDraw): RuleDraw {
  const draw = input as RuleDraw & Record<string, unknown>;
  return {
    regular: intValues(draw.regular as LooseArray<number>, "draw.regular"),
    special: intValues(draw.special as LooseArray<number>, "draw.special"),
    digits: intValues(draw.digits as LooseArray<number>, "draw.digits"),
  };
}

function validateSelection(
  definition: RuleDefinition,
  selection: RuleTicketSelection,
) {
  const { model, selection: rule } = definition;
  const regularPool = poolValues(definition, false);
  const specialPool = poolValues(definition, true);
  const allFieldsEmpty = (except: keyof RuleTicketSelection) => {
    const keys: (keyof RuleTicketSelection)[] = [
      "regular",
      "special",
      "digits",
      "exclude",
      "attributes",
      "features",
    ];
    return keys
      .filter((key) => key !== except)
      .every((key) => {
        const value = selection[key];
        return Array.isArray(value)
          ? value.length === 0
          : Object.keys(value).length === 0;
      });
  };

  if (rule.mode === "numbers") {
    if (model.model === "DIGITS_0_9") {
      if (
        selection.regular.length ||
        selection.special.length ||
        selection.exclude.length ||
        Object.keys(selection.attributes).length ||
        Object.keys(selection.features).length ||
        selection.digits.length !== model.length
      )
        fail("Digit selections must contain one candidate list per position.");
      for (const [index, values] of selection.digits.entries()) {
        if (values.length > 10) fail("数字候选每位最多 10 个输入值。");
        assertCount(values, 1, `selection.digits[${index}]`, 10);
        if (values.some((value) => value > 9))
          fail("Digit candidates must be from 0 to 9.");
      }
      if (!model.allow_repeat && !distinctAssignmentExists(selection.digits))
        fail(
          "Digit position candidates cannot produce a draw without repeats.",
        );
      return;
    }
    if (
      selection.digits.length ||
      selection.exclude.length ||
      Object.keys(selection.attributes).length ||
      Object.keys(selection.features).length
    )
      fail("Number selections contain fields that do not apply to this mode.");
    const regularCount = rule.regular_count;
    const specialCount = rule.special_count;
    for (const [values, count, pool, label, repeat] of [
      [
        selection.regular,
        regularCount,
        regularPool,
        "Regular candidates",
        model.regular_pool.allow_repeat,
      ],
      [
        selection.special,
        specialCount,
        specialPool,
        "Special candidates",
        model.special_pool.allow_repeat,
      ],
    ] as const) {
      if (count === 0) {
        if (values.length)
          fail(`${label} must be empty when its configured count is zero.`);
        continue;
      }
      if (values.length > 100)
        fail(`${label} contains more than 100 input values.`);
      assertCount([...new Set(values)], repeat ? 1 : count, label);
      const allowed = new Set(pool);
      if (values.some((value) => !allowed.has(value)))
        fail(`${label} contain values outside their pool.`);
    }
    if (model.model === "M_SELECT_N") {
      const union = new Set([...selection.regular, ...selection.special]);
      if (union.size < regularCount + specialCount)
        fail(
          "M-select candidates cannot supply distinct regular and special values.",
        );
    }
    return;
  }

  if (rule.mode === "exclude") {
    if (!allFieldsEmpty("exclude"))
      fail("Exclude selection contains fields that do not apply to this mode.");
    unique(selection.exclude, "Excluded candidates");
    if (
      selection.exclude.length !== rule.exclude_count ||
      selection.exclude.length > 100
    )
      fail(
        `Excluded candidates must contain exactly ${rule.exclude_count} values.`,
      );
    const allowed = new Set(
      model.model === "DIGITS_0_9"
        ? Array.from({ length: 10 }, (_, index) => index)
        : [...regularPool, ...specialPool],
    );
    if (selection.exclude.some((value) => !allowed.has(value)))
      fail("Excluded candidates contain values outside the model pools.");
    return;
  }

  if (rule.mode === "attributes") {
    if (!allFieldsEmpty("attributes"))
      fail(
        "Attribute selection contains fields that do not apply to this mode.",
      );
    const groups = rule.attribute_groups ?? [];
    if (
      Object.keys(selection.attributes).length !== groups.length ||
      groups.some((group) => !Object.hasOwn(selection.attributes, group))
    )
      fail("Select values for every configured attribute group.");
    for (const group of groups) {
      const values = selection.attributes[group]!;
      uniqueStrings(values, `Attribute ${group}`);
      if (values.length < 1 || values.length > 100)
        fail(`Select at least one value for attribute ${group}.`);
      const allowed = definition.number_attributes?.[group] ?? {};
      if (values.some((value) => !Object.hasOwn(allowed, value)))
        fail(`Attribute ${group} contains an unavailable value.`);
    }
    return;
  }

  if (rule.mode === "features") {
    if (!allFieldsEmpty("features"))
      fail("Feature selection contains fields that do not apply to this mode.");
    const choices = rule.feature_choices ?? {};
    const keys = Object.keys(choices);
    if (
      Object.keys(selection.features).length !== keys.length ||
      keys.some((key) => !Object.hasOwn(selection.features, key))
    )
      fail("Select values for every configured feature.");
    for (const key of keys) {
      const values = selection.features[key]!;
      unique(values, `Feature ${key}`);
      assertCount(values, 1, `Feature ${key}`);
      const allowed = new Set(choices[key] ?? []);
      if (
        values.some((value) => !allowed.has(value) || value > MAX_FEATURE_VALUE)
      )
        fail(`Feature ${key} contains an unavailable value.`);
    }
    return;
  }
  fail("Unsupported selection mode.");
}

function uniqueStrings(values: string[], field: string) {
  if (new Set(values).size !== values.length)
    fail(`${field} cannot contain duplicate values.`);
}

/** Convert only Go-style nil collections to empty collections without changing scalar rule inputs. */
export function normalizeRuleCase(
  input: RuleValidationCase,
): RuleValidationCase {
  const source = input as RuleValidationCase & {
    selection?: Partial<RuleTicketSelection> | null;
    draw?: Partial<RuleDraw> | null;
  };
  const selection = source.selection ?? {};
  const draw = source.draw ?? {};
  const attributes = selection.attributes ?? {};
  const features = selection.features ?? {};
  return {
    ...source,
    selection: {
      regular: selection.regular ?? [],
      special: selection.special ?? [],
      digits: (selection.digits ?? []).map((position) => position ?? []),
      exclude: selection.exclude ?? [],
      attributes: Object.fromEntries(
        Object.entries(attributes).map(([key, values]) => [key, values ?? []]),
      ),
      features: Object.fromEntries(
        Object.entries(features).map(([key, values]) => [key, values ?? []]),
      ),
    },
    draw: {
      regular: draw.regular ?? [],
      special: draw.special ?? [],
      digits: draw.digits ?? [],
    },
  };
}

export function validateRuleCase(
  definition: RuleDefinition,
  input: RuleValidationCase,
): void {
  if (!input || typeof input !== "object")
    fail("A validation case is required.");
  const item = normalizeRuleCase(input);
  if (
    typeof item.name !== "string" ||
    !item.name.trim() ||
    item.name.includes("\0") ||
    !isValidUtf8String(item.name) ||
    utf8Length(item.name) > 120
  )
    fail(
      "Case names must be valid UTF-8, non-empty, NUL-free and at most 120 bytes.",
    );
  canonicalInt64(item.expected_bet_points, "Expected bet points");
  canonicalInt64(item.expected_prize_points, "Expected prize points");
  if (typeof item.expected_won !== "boolean")
    fail("Expected won must be true or false.");
  const multiplier = canonicalInt64(item.multiplier, "Multiplier", 1n);
  const maxMultiplier = canonicalInt64(
    definition.limits?.max_multiplier,
    "Definition maximum multiplier",
    1n,
  );
  if (multiplier > maxMultiplier)
    fail("Multiplier exceeds the definition limit.");

  const model = definition.model;
  if (!model || !definition.selection)
    fail("A complete rule definition is required.");
  if (model.model === "X_PLUS_Y" && model.allow_repeat)
    fail(
      "X_PLUS_Y model.allow_repeat must be false; repeat behavior belongs to each pool.",
    );
  if (
    model.model === "M_SELECT_N" &&
    (model.allow_repeat ||
      model.regular_pool.allow_repeat ||
      model.special_pool.allow_repeat)
  )
    fail("M-select pools and model cannot allow repeated numbers.");
  if (
    model.model !== "X_PLUS_Y" &&
    model.model !== "M_SELECT_N" &&
    model.model !== "DIGITS_0_9"
  )
    fail("Unsupported rule model.");
  if (definition.selection.mode === "numbers") {
    if (
      model.model === "DIGITS_0_9" &&
      (definition.selection.regular_count !== 0 ||
        definition.selection.special_count !== 0)
    )
      fail("Digit number selection cannot use regular or special counts.");
    if (
      model.model !== "DIGITS_0_9" &&
      definition.selection.regular_count + definition.selection.special_count <
        1
    )
      fail(
        "Number selection must choose at least one regular or special number.",
      );
  }
  const regularPool = poolValues(definition, false);
  const specialPool = poolValues(definition, true);
  const draw = drawOf(item.draw);
  if (model.model === "DIGITS_0_9") {
    if (
      draw.regular.length ||
      draw.special.length ||
      draw.digits.length !== model.length
    )
      fail(`Digit draw must contain exactly ${model.length} digits.`);
    if (draw.digits.some((digit) => digit > 9))
      fail("Draw digits must be from 0 to 9.");
    if (!model.allow_repeat) unique(draw.digits, "Draw digits");
  } else {
    if (draw.digits.length)
      fail("A number draw cannot contain digit positions.");
    drawPool(
      regularPool,
      model.regular_count,
      model.regular_pool.allow_repeat,
      "Regular draw pool",
    );
    drawPool(
      specialPool,
      model.special_count,
      model.special_pool.allow_repeat,
      "Special draw pool",
    );
    assertDrawValues(
      draw.regular,
      model.regular_count,
      regularPool,
      model.regular_pool.allow_repeat,
      "Regular draw",
    );
    assertDrawValues(
      draw.special,
      model.special_count,
      specialPool,
      model.special_pool.allow_repeat,
      "Special draw",
    );
    if (
      model.model === "M_SELECT_N" &&
      draw.regular.some((value) => draw.special.includes(value))
    )
      fail("M-select regular and special draw values must be distinct.");
  }
  validateSelection(definition, selectionOf(item.selection));
}

export function validateRuleCases(
  definition: RuleDefinition,
  cases: RuleValidationCase[],
): void {
  if (!Array.isArray(cases) || cases.length < 1 || cases.length > 32)
    fail("Provide between 1 and 32 validation cases.");
  const names = new Set<string>();
  for (const item of cases) {
    validateRuleCase(definition, item);
    const key = item.name.trim();
    if (names.has(key)) fail("Case names must be unique.");
    names.add(key);
  }
}

function firstPoolValues(
  definition: RuleDefinition,
  special: boolean,
  count: number,
): number[] {
  const pool = special
    ? definition.model.special_pool
    : definition.model.regular_pool;
  const values = poolValues(definition, special);
  if (!pool.allow_repeat && values.length < count)
    fail("The definition pool cannot satisfy the draw count.");
  if (!values.length && count) fail("The definition pool is empty.");
  return Array.from(
    { length: count },
    (_, index) => values[index % values.length]!,
  );
}

export function defaultRuleCase(
  definition: RuleDefinition,
  index = 1,
): RuleValidationCase {
  const { model, selection: rule } = definition;
  const selection: RuleTicketSelection = {
    regular: [],
    special: [],
    digits: [],
    exclude: [],
    attributes: {},
    features: {},
  };
  const draw: RuleDraw = { regular: [], special: [], digits: [] };
  if (model.model === "DIGITS_0_9") {
    draw.digits = Array.from({ length: model.length }, (_, position) =>
      model.allow_repeat ? 0 : position,
    );
    if (rule.mode === "numbers")
      selection.digits = Array.from({ length: model.length }, (_, position) => [
        model.allow_repeat ? 0 : position,
      ]);
  } else {
    const regularValues = poolValues(definition, false);
    const specialValues = poolValues(definition, true);
    if (model.model === "M_SELECT_N") {
      const count = model.regular_count + model.special_count;
      const source = regularValues;
      if (new Set(source).size < count)
        fail("M-select definition pool cannot supply distinct draw values.");
      draw.regular = source.slice(0, model.regular_count);
      draw.special = source.slice(model.regular_count, count);
      if (rule.mode === "numbers") {
        const selectionValues = source.slice(0, count);
        selection.regular = selectionValues.slice(0, rule.regular_count);
        selection.special = selectionValues.slice(
          rule.regular_count,
          rule.regular_count + rule.special_count,
        );
      }
    } else {
      draw.regular = firstPoolValues(definition, false, model.regular_count);
      draw.special = firstPoolValues(definition, true, model.special_count);
      if (rule.mode === "numbers") {
        selection.regular = regularValues.slice(0, rule.regular_count);
        selection.special = specialValues.slice(0, rule.special_count);
      }
    }
  }

  if (rule.mode === "exclude") {
    const allowed =
      model.model === "DIGITS_0_9"
        ? Array.from({ length: 10 }, (_, value) => value)
        : [...poolValues(definition, false), ...poolValues(definition, true)];
    selection.exclude = [...new Set(allowed)].slice(0, rule.exclude_count);
  } else if (rule.mode === "attributes") {
    for (const group of rule.attribute_groups ?? []) {
      const first = Object.keys(definition.number_attributes?.[group] ?? {})[0];
      if (first === undefined)
        fail(`Attribute ${group} has no selectable value.`);
      selection.attributes[group] = [first];
    }
  } else if (rule.mode === "features") {
    for (const [key, values] of Object.entries(rule.feature_choices ?? {})) {
      if (!values.length) fail(`Feature ${key} has no selectable value.`);
      selection.features[key] = [values[0]!];
    }
  }

  const result: RuleValidationCase = {
    name: `Validation case ${index}`,
    selection,
    draw,
    multiplier: "1",
    expected_bet_points: "0",
    expected_prize_points: "0",
    expected_won: false,
  };
  validateRuleCase(definition, result);
  return result;
}
