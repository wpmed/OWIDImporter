import { useCallback, useState } from "react"
import { SESSION_ID_KEY } from "../constants";

export enum SocketMessageTypeEnum {
  WIKITEXT = "wikitext",
  PROGRESS = "progress",
  MSG = "msg",
  ERROR = "error",
  TASK_PROCESS = "task_process",
  TASK = "task"
}

export interface SocketMessage {
  type: SocketMessageTypeEnum,
  msg: string
}

export const useWebsocket = () => {
  const [ws, setWS] = useState<WebSocket | null>(null);
  const [messages, setMessages] = useState<SocketMessage[]>([]);




  const connect = useCallback(() => {
    console.log("Connecting WS");
    const sessionId = window.localStorage.getItem(SESSION_ID_KEY);
    const url = import.meta.env.VITE_BASE_URL + `/ws?${SESSION_ID_KEY}=${sessionId}`;

    const ws = new WebSocket(
      url
    );

    ws.addEventListener("open", () => {
      // console.log("SOCKET CONNECTED");
      setWS(ws);
    });

    ws.addEventListener("close", (e) => {
      if (e.code == 1000 && e.wasClean) {
        setMessages((val) => { return val.concat({ msg: "Connection closed", type: SocketMessageTypeEnum.MSG }); })
      } else {
        setMessages((val) => {
          return val.concat(

            {
              msg: "Connection error (" + e.code + e.reason + ")",
              type: SocketMessageTypeEnum.ERROR,
            }
          );
        })
      }
    });
    // ws.addEventListener("message", (event) => {
    //   const info = JSON.parse(event.data) as SocketMessage;
    //   setMessages((val) => { return val.concat(info); })
    // });
  }, [setWS, setMessages])

  const disconnect = useCallback(() => {
    setWS(ws => {
      if (ws) {
        ws.close(1000)
        console.log("Socket disconnected");
      }

      return null;
    });
  }, [setWS])

  // console.log({ messages });
  return {
    ws,
    messages,
    connect,
    disconnect
  }
}
