export enum TaskStatusEnum {
  Queued = "queued",
  Processing = "processing",
  Done = "done",
  Retrying = "retrying",
  Failed = "failed",
}

export enum TaskProcessStatusEnum {
  Processing = "processing",
  Uploaded = "uploaded",
  Overwritten = "overwritten",
  Skipped = "skipped",
  DescriptionUpdated = "description_updated",
  Retrying = "retrying",
  Failed = "failed",
}

export interface TaskProcess {
  id: string,
  region: string,
  year: number,
  status: TaskProcessStatusEnum,
  taskId: string,
  filename: string
}

export enum TaskTypeEnum {
  MAP = "map",
  CHART = "chart"
}

export enum DescriptionOverwriteBehaviour {
  ALL = "all",
  ALL_EXCEPT_CATEGORIES = "all_except_categories"
}

export interface Task {
  id: string,
  userId: string,
  url: string,
  filename: string,
  description: string,
  descriptionOverwriteBehaviour: DescriptionOverwriteBehaviour,
  chartName: string,
  status: TaskStatusEnum,
  type: TaskTypeEnum,
  lastOperationAt: number,
  createdAt: number,
}

