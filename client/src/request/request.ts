import { SESSION_ID_KEY } from "../constants";
import { DescriptionOverwriteBehaviour, Task, TaskProcess, TaskTypeEnum } from "../types";

export interface ReplaceSessionResponse {
  sessionId: string,
  username: string
}

export interface VerifySessionResponse {
  error: string,
  username: string
}


export async function replaceSession(sessionId: string) {
  const response = await fetch(import.meta.env.VITE_BASE_URL + "/session/replace", {
    method: "POST",
    body: JSON.stringify({ sessionId }),
    headers: {
      "Content-Type": "application/json"
    }
  });

  const data = await response.json() as ReplaceSessionResponse;

  return data;
}

export async function verifySession(sessionId: string) {
  const response = await fetch(import.meta.env.VITE_BASE_URL + "/session/verify", {
    method: "POST",
    body: JSON.stringify({ sessionId }),
    headers: {
      "Content-Type": "application/json"
    }
  });

  const data = await response.json() as VerifySessionResponse;

  return data;
}

export async function logout() {
  const sessionId = window.localStorage.getItem(SESSION_ID_KEY)!;

  await fetch(`${import.meta.env.VITE_BASE_URL}/logout`, {
    method: "GET",
    headers: {
      "Content-Type": "application/json",
      ...(sessionId ? {
        [SESSION_ID_KEY]: sessionId
      } : {})
    }
  });
}

export interface CreateTaskData {
  action: string,
  url: string,
  fileName: string,
  description: string,
  descriptionOverwriteBehaviour: DescriptionOverwriteBehaviour
}

export interface CreateTaskResponse {
  error?: string,
  taskId?: string
}

export async function createTask(data: CreateTaskData) {
  const sessionId = window.localStorage.getItem(SESSION_ID_KEY)!;
  const response = await fetch(import.meta.env.VITE_BASE_URL + "/task", {
    method: "POST",
    body: JSON.stringify(data),
    headers: {
      "Content-Type": "application/json",
      [SESSION_ID_KEY]: sessionId
    }
  });

  const responseData = await response.json() as CreateTaskResponse;

  return responseData;
}

export async function retryTask(id: string) {
  const sessionId = window.localStorage.getItem(SESSION_ID_KEY)!;
  const response = await fetch(`${import.meta.env.VITE_BASE_URL}/task/${id}/retry`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      [SESSION_ID_KEY]: sessionId
    }
  });

  const responseData = await response.json() as CreateTaskResponse;

  return responseData;
}

export async function cancelTask(id: string) {
  const sessionId = window.localStorage.getItem(SESSION_ID_KEY)!;
  const response = await fetch(`${import.meta.env.VITE_BASE_URL}/task/${id}/cancel`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      [SESSION_ID_KEY]: sessionId
    }
  });

  const responseData = await response.json() as CreateTaskResponse;

  return responseData;
}

interface FetchTasksResopnse {
  tasks?: Task[]
  error?: string
}
export async function fetchTasks(taskType: TaskTypeEnum) {
  const sessionId = window.localStorage.getItem(SESSION_ID_KEY)!;
  const response = await fetch(`${import.meta.env.VITE_BASE_URL}/task?taskType=${taskType}`, {
    method: "GET",
    headers: {
      "Content-Type": "application/json",
      ...(sessionId ? {
        [SESSION_ID_KEY]: sessionId
      } : {})
    }
  });

  const responseData = await response.json() as FetchTasksResopnse;

  return responseData;
}

export interface FetchTaskByIdResponse {
  task: Task,
  processes: TaskProcess[],
  wikiText?: string
}

export async function fetchTaskById(id: string) {
  const sessionId = window.localStorage.getItem(SESSION_ID_KEY)!;

  const response = await fetch(`${import.meta.env.VITE_BASE_URL}/task/${id}`, {
    method: "GET",
    headers: {
      "Content-Type": "application/json",
      ...(sessionId ? {
        [SESSION_ID_KEY]: sessionId
      } : {})
    }
  });

  const responseData = await response.json() as FetchTaskByIdResponse;

  return responseData;
}

