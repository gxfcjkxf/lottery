export function validAgentPercentage(value: string): boolean {
  return /^\d{1,3}(?:\.\d{1,2})?$/.test(value) && Number(value) <= 100;
}

export function agentPercentageToRatio(value: string): string {
  if (!validAgentPercentage(value)) throw new RangeError('Invalid agent percentage');
  const [whole, fraction = ''] = value.split('.');
  const basisPoints = Number(whole) * 100 + Number(fraction.padEnd(2, '0'));
  return String(basisPoints / 10_000);
}

export function agentRatioToPercentage(value: string): string {
  const [whole, fraction = ''] = value.split('.');
  const micros = Number(whole) * 1_000_000 + Number(fraction.padEnd(6, '0'));
  const decimals = String(micros % 10_000).padStart(4, '0').replace(/0+$/, '');
  return `${Math.floor(micros / 10_000)}${decimals ? `.${decimals}` : ''}`;
}
