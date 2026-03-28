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

export enum SocketMessageActionEnum {
  SUBSCRIBE_TASK = "subscribe_task",
  UNSUBSCRIBE_TASK = "unsubscribe_task",

  SUBSCRIBE_TASK_LIST = "subscribe_task_list",
  UNSUBSCRIBE_TASK_LIST = "unsubscribe_task_list",
}

export interface SocketMessage {
  type: SocketMessageTypeEnum,
  msg: string
}

export const useWebsocket = () => {
  const [ws, setWS] = useState<WebSocket | null>(null);
  const [messages, setMessages] = useState<SocketMessage[]>([]);




  const connect = useCallback(() => {
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
