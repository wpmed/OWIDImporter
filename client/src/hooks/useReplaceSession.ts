import { useEffect, useState } from "react";
import { replaceSession, verifySession } from "../request/request";
import { SESSION_ID_KEY, USERNAME_KEY } from "../constants";
import { consumeReturnPath } from "../routing";

export const useReplaceSession = () => {
  // Read during render: child route effects (e.g. the "/" redirect) run before
  // this effect and may already have dropped the ?sessionId= query.
  const [search] = useState(() => window.location.search);

  useEffect(() => {

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
                // Back to the page the user was on before logging in (a shared link)
                window.location.href = consumeReturnPath();
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
            // Reload the same page: a public shared link stays open, a private page shows the login
            window.location.reload();
          })
      }
    }
  }, [search])
}
