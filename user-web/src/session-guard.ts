import type { NavigationGuard } from 'vue-router';

const publicPaths = new Set(['/login', '/register', '/terms', '/privacy']);

/** Resolve the real cookie session before mounting any business page. */
export function createUserSessionGuard(check: () => Promise<unknown>): NavigationGuard {
  return async to => {
    if (publicPaths.has(to.path)) return true;
    try {
      await check();
      return true;
    } catch (cause) {
      if (cause && typeof cause === 'object' && 'status' in cause && cause.status === 401) {
        return { path: '/login', query: to.query, replace: true };
      }
      throw cause;
    }
  };
}
