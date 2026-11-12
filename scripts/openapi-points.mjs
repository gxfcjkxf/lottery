// Exact fixed-point int64 bounds, expressed for decimal JSON strings.
function wholeRange(limit) {
  const alternatives = ['0', `[1-9][0-9]{0,${limit.length - 2}}`, limit]
  for (let i = 0; i < limit.length; i++) {
    const floor = i === 0 ? 1 : 0, ceiling = Number(limit[i]) - 1
    if (ceiling < floor) continue
    const digit = ceiling === floor ? String(floor) : `[${floor}-${ceiling}]`
    alternatives.push(limit.slice(0, i) + digit + `[0-9]{${limit.length - i - 1}}`)
  }
  return `(?:${alternatives.join('|')})`
}
const ordinary = `${wholeRange('92233720368547757')}(?:[.][0-9]{1,2})?`
const positive = `(?:${ordinary}|92233720368547758(?:[.]0[0-7]?)?)`
const negative = `(?:${ordinary}|92233720368547758(?:[.]0[0-8]?)?)`
export const pointSchemas = {
  Points: { type: 'string', pattern: `^(?:${positive}|-${negative})$`, not: { enum: ['-0','-0.0','-0.00'] }, description: 'Exact points in -92233720368547758.08..92233720368547758.07, with at most two fractional digits.' },
  NonnegativePoints: { type: 'string', pattern: `^${positive}$`, description: 'Exact nonnegative points with at most two fractional digits, up to 92233720368547758.07.' },
  PositivePoints: { type: 'string', pattern: `^${positive}$`, not: { enum: ['0','0.0','0.00'] }, description: 'Exact positive points with at most two fractional digits, up to 92233720368547758.07.' },
}

// Apply decimal precision to custom monetary schema properties; counts remain integers.
export function applyPointsPrecision(doc) {
  const monetary = key => key === 'points' || key.endsWith('_points') || key.startsWith('points_') || key.endsWith('_available') || ['available','manual_frozen','system_frozen','withdrawal'].includes(key)
  function visit(value) {
    if (!value || typeof value !== 'object') return
    if (value.properties) for (const [key, schema] of Object.entries(value.properties)) {
      if (!monetary(key)) continue
      function amount(node) {
        if (node.$ref && /\/(Int64String|NonnegativeInt64String|PositiveInt64String)$/.test(node.$ref)) {
          node.$ref = node.$ref.replace(/(Nonnegative|Positive)?Int64String$/, '$1Points')
        } else if (node.type === 'string' && node.pattern && /\[0-9\]/.test(node.pattern)) {
          node.pattern = key === 'net_points' || key === 'delta_points' || (node.pattern.startsWith('^-') || node.pattern.includes('|-')) ? '^-?(0|[1-9][0-9]*)([.][0-9]{1,2})?$' : '^(0|[1-9][0-9]*)([.][0-9]{1,2})?$'
          if (node.description) node.description = node.description.replace(/decimal integer string|integer string/g, 'decimal points string').replace(/Exact signed int64 decimal string/g, 'Exact decimal points string')
        }
        for (const child of node.anyOf ?? []) amount(child)
      }
      amount(schema)
    }
    for (const child of Object.values(value)) if (Array.isArray(child)) child.forEach(visit); else visit(child)
  }
  visit(doc)
  return doc
}
