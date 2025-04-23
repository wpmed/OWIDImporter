import { useEffect } from "react";
import { replaceSession, verifySession } from "../request/request";
import { SESSION_ID_KEY, USERNAME_KEY } from "../constants";

export const useReplaceSession = () => {
  useEffect(() => {
    const search = window.location.search;

    if (search.includes(SESSION_ID_KEY)) {
      const parts = search.replace("?", "").split("&");
      parts.forEach(part => {
        const [key, val] = part.split("=");
        if (key == SESSION_ID_KEY) {
          replaceSession(val)
            .then(res => {
              console.log("Replace session response", res);
              if (res.sessionId) {
                window.localStorage.setItem(SESSION_ID_KEY, res.sessionId);
                window.localStorage.setItem(USERNAME_KEY, res.username);
                window.location.href = "/";
              }
            }).catch(err => {
              console.log("Error replacing session", err);
            })
        }
      });
    } else {
      // Verify session is valid
      const sessionId = window.localStorage.getItem(SESSION_ID_KEY);
      if (sessionId) {
        verifySession(sessionId)
          .then((res) => {
            if (res.error) {
              throw new Error(res.error);
            }
            window.localStorage.setItem(USERNAME_KEY, res.username);
          }).catch(err => {
            console.log("Error verifying session", err);
            window.localStorage.removeItem(SESSION_ID_KEY);
            window.localStorage.removeItem(USERNAME_KEY);
            window.location.href = "/";
          })
      }
    }
  }, [])
}
