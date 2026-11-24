export interface LatestLookupHandlers<T> {
  /** The lookup's result, only while its URL is still the current one. */
  result: (value: T) => void
  /** The lookup's error, only while its URL is still the current one (never for an aborted lookup). */
  error: (err: unknown) => void
  /** Always, once the lookup settles: whether it is still the newest lookup, and whether its URL is still current. */
  settled: (newest: boolean, current: boolean) => void
}

/**
 * Runs URL lookups of which only the newest counts. Starting one aborts the one before it, and
 * a lookup's result or error is passed on only while its URL is still `currentUrl()`, so a slow
 * response for an older URL can never overwrite the answer for the URL now in the field.
 */
export function createLatestLookup(currentUrl: () => string) {
  let controller: AbortController | null = null;
  let lastId = 0;
  return function lookup<T>(url: string, fetch: (signal: AbortSignal) => Promise<T>, on: LatestLookupHandlers<T>) {
    const id = ++lastId;
    controller?.abort();
    const own = new AbortController();
    controller = own;
    const applies = () => !own.signal.aborted && currentUrl() === url;
    let pending: Promise<T>;
    try {
      pending = fetch(own.signal);
    } catch (err) {
      pending = Promise.reject(err);
    }
    pending
      .then(value => {
        if (applies()) on.result(value);
      }, err => {
        if (applies()) on.error(err);
      })
      .finally(() => on.settled(id === lastId, currentUrl() === url));
  };
}
