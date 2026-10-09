import { test } from '@playwright/test';

// CI isolates each viewport in its own database; local combined runs must also
// isolate login counters rather than disable production authentication limits.
export function reviewPlatformUsername() {
  const project = test.info().project.name;
  if (project !== 'desktop1440' && project !== 'mobile360') throw new Error(`Unsupported review viewport: ${project}`);
  return `review_platform_${project}`;
}
