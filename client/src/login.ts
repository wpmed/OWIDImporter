import { saveReturnPath } from "./routing";

export const LOGIN_URL = import.meta.env.VITE_BASE_URL + "/login";

// Remembers the current page so the user comes back to it after logging in.
export const rememberCurrentPage = () => {
  saveReturnPath(window.location.pathname + window.location.search);
}
