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

const API_BASE = import.meta.env.VITE_BASE_URL;

export async function replaceSession(sessionId: string) {
  const response = await fetch(API_BASE + "/session/replace", {
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
  const response = await fetch(API_BASE + "/session/verify", {
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

  await fetch(`${API_BASE}/logout`, {
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
  chartParameters: string, // Query string for the chart parameters
  fileName: string,
  description: string,
  descriptionOverwriteBehaviour: DescriptionOverwriteBehaviour
  importCountries?: boolean,
  generateTemplateCommons?: boolean,
  countryFileName?: string,
  countryDescription?: string,
  countryDescriptionOverwriteBehaviour?: DescriptionOverwriteBehaviour,
  templateNameFormat: string
}

export interface CreateTaskResponse {
  error?: string,
  taskId?: string
}

export async function createTask(data: CreateTaskData) {
  const sessionId = window.localStorage.getItem(SESSION_ID_KEY)!;
  const response = await fetch(API_BASE + "/task", {
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
  const response = await fetch(`${API_BASE}/task/${id}/retry`, {
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
  const response = await fetch(`${API_BASE}/task/${id}/cancel`, {
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
  const response = await fetch(`${API_BASE}/task?taskType=${taskType}`, {
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

  const response = await fetch(`${API_BASE}/task/${id}`, {
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

export interface GetChartParametersResponse {
  params: ChartParamteres[]
  info: ChartInfo
}

export interface ChartInfo {
  params: ChartParamteres[]
  title: string
  hasCountries: boolean
  startYear: string
  endYear: string
}

export interface ChartParamteres {
  name: string
  description: string
  slug: string
  choices: ChartParamteresChoice[]
}

export interface ChartParamteresChoice {
  name: string
  slug: string
}

export async function getChartParameters(url: string) {
  const sessionId = window.localStorage.getItem(SESSION_ID_KEY)!;

  const response = await fetch(`${API_BASE}/chart/parameters?url=${url}`, {
    method: "GET",
    headers: {
      "Content-Type": "application/json",
      ...(sessionId ? {
        [SESSION_ID_KEY]: sessionId
      } : {})
    }
  });

  const responseData = await response.json() as GetChartParametersResponse;

  return responseData;
}
