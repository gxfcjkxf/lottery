import { readApiJson } from './association-names'

type ErrorFactory = (message: string, status: number, code: string) => Error
export interface ApiResponse<T> { success: true; data: T; [key: string]: unknown }
const record = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value)
const invalid = (error: ErrorFactory) => error('Invalid server response; operation result is unconfirmed', 502, 'INVALID_RESPONSE')

function rejection(value: unknown, response: Response, error: ErrorFactory): Error {
  if (!response.ok && record(value) && value.success === false && record(value.error) &&
      typeof value.error.code === 'string' && /^[A-Za-z][A-Za-z0-9_]*$/.test(value.error.code) &&
      typeof value.error.message === 'string' && value.error.message.trim()) {
    return error(value.error.message, response.status, value.error.code)
  }
  return invalid(error)
}

/** Malformed responses never confirm a business rejection or a successful write. */
export async function readApiEnvelope<T = unknown>(response: Response, error: ErrorFactory): Promise<ApiResponse<T>> {
  let value: unknown
  try { value = await readApiJson(response) } catch { throw invalid(error) }
  if (!response.ok || !record(value) || value.success !== true || (!Object.hasOwn(value, 'data') || value.data === undefined)) throw rejection(value, response, error)
  return value as unknown as ApiResponse<T>
}

export async function readApiRejection(response: Response, error: ErrorFactory): Promise<Error> {
  let value: unknown
  try { value = await readApiJson(response) } catch { return invalid(error) }
  return rejection(value, response, error)
}
