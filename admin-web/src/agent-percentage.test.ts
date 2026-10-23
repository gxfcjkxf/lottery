import { describe, expect, it } from 'vitest';
import { agentPercentageToRatio, agentRatioToPercentage, validAgentPercentage } from './agent-percentage';

describe('agent percentage inputs', () => {
  it.each([['20', '0.2'], ['0', '0'], ['100', '1'], ['100.00', '1'], ['12.30', '0.123'], ['0.01', '0.0001'], ['29', '0.29'], ['99.99', '0.9999']])('converts %s percent to the exact API ratio %s', (percentage, ratio) => {
    expect(validAgentPercentage(percentage)).toBe(true);
    expect(agentPercentageToRatio(percentage)).toBe(ratio);
    expect(agentRatioToPercentage(ratio)).toBe(String(Number(percentage)));
  });
  it.each(['', '-1', '333', '100.01', '1.234', '1e2', '2%', '.5'])('rejects invalid percentage %s', value => {
    expect(validAgentPercentage(value)).toBe(false);
    expect(() => agentPercentageToRatio(value)).toThrow(RangeError);
  });
  it('does not round existing saved ratios when opening an editor', () => {
    expect(agentRatioToPercentage('0.123456')).toBe('12.3456');
    expect(validAgentPercentage(agentRatioToPercentage('0.123456'))).toBe(false);
  });
});
