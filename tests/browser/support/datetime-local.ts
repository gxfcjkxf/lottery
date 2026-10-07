// Keep this function self-contained: page.evaluate serializes it and runs it
// in the browser's timezone, which may differ from the Node worker's timezone.
export function formatDateTimeLocal(timestamp: number): string {
  const date = new Date(timestamp);
  const pad = (value: number) => String(value).padStart(2, "0");
  const minute = `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
  // Native datetime-local controls canonicalize :00 away. Playwright checks
  // the assigned value exactly, so preserve nonzero seconds but omit zero.
  return date.getSeconds() === 0 ? minute : `${minute}:${pad(date.getSeconds())}`;
}
