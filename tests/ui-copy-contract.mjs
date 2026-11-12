// Instructions belong in control constraints and validation, not label suffixes.
// Units, state distinctions, range boundaries and blank-value behavior remain valid.
export const redundantAnnotation = /[（(][^()（）]*(?:可选|选填|必填|最多\s*\d+|最少\s*\d+|\d+\s*(?:个字符|字符|characters?)|逗号分隔|明细|\boptional\b|\brequired\b|\b(?:max(?:imum)?|min(?:imum)?)\s+\d+|comma[ -]separated|\bdetails\b|numerator\s*\/\s*denominator)[^()（）]*[）)]/i
