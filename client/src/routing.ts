// Client routes. Paths are plural so they never collide with the API GETs
// (/task/:id, /operation/:id) served by the same server.
export const PATHS = {
  MAPS: "/maps",
  NEW_MAP: "/maps/new",
  IFRAMES: "/iframes",
  NEW_IFRAME: "/iframes/new",
  OPERATIONS: "/operations",
  NEW_OPERATION: "/operations/new",
} as const;

export const mapPath = (id: string) => `${PATHS.MAPS}/${encodeURIComponent(id)}`;
export const iframePath = (id: string) => `${PATHS.IFRAMES}/${encodeURIComponent(id)}`;
export const operationPath = (id: string) => `${PATHS.OPERATIONS}/${encodeURIComponent(id)}`;

// A sidebar entry is either a section list (active on the list and its detail
// pages) or a "new" form (active only on its own path).
export type NavMatch = { list: string, newPath: string } | { exact: string };

export function isNavActive(match: NavMatch, pathname: string): boolean {
  const path = pathname.length > 1 ? pathname.replace(/\/+$/, "") : pathname;
  if ("exact" in match) {
    return path === match.exact;
  }
  if (path === match.list) return true;
  return path.startsWith(match.list + "/") && path !== match.newPath;
}

export const RETURN_PATH_KEY = "returnPath";

// Only same-origin absolute paths are accepted, so a stored value can never
// send the user to another site after login.
export function isSafeReturnPath(path: string | null | undefined): path is string {
  return !!path && path.startsWith("/") && !path.startsWith("//") && !path.startsWith("/\\");
}

// Remembers where the user was before going to the OAuth login.
export function saveReturnPath(path: string, storage: Pick<Storage, "setItem" | "removeItem"> = window.localStorage) {
  if (isSafeReturnPath(path)) {
    storage.setItem(RETURN_PATH_KEY, path);
  } else {
    storage.removeItem(RETURN_PATH_KEY);
  }
}

// Returns the remembered path (or "/") and forgets it.
export function consumeReturnPath(storage: Pick<Storage, "getItem" | "removeItem"> = window.localStorage): string {
  const path = storage.getItem(RETURN_PATH_KEY);
  storage.removeItem(RETURN_PATH_KEY);
  return isSafeReturnPath(path) ? path : "/";
}
