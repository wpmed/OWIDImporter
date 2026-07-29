export enum TaskStatusEnum {
  Queued = "queued",
  Processing = "processing",
  Done = "done",
  Retrying = "retrying",
  Failed = "failed",
  Cancelled = "cancelled",
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
  date: string,
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
  ALL_EXCEPT_CATEGORIES = "all_except_categories",
  ONLY_FILE = "only_file",
  SKIP = "skip"
}

export interface Task {
  id: string,
  userId: string,
  url: string,
  filename: string,
  description: string,
  commonsTemplateNameFormat?: string
  descriptionOverwriteBehaviour: DescriptionOverwriteBehaviour,
  countryFileName?: string,
  countryDescription?: string,
  countryDescriptionOverwriteBehaviour?: DescriptionOverwriteBehaviour,
  importCountries: number,
  archived: number,
  generateTemplateCommons: number,
  chartName: string,
  commonsTemplateName?: string,
  status: TaskStatusEnum,
  type: TaskTypeEnum,
  lastOperationAt: number,
  createdAt: number,
}



export enum OperationTypeEnum {
  UPDATE_DEFAULTS = "update_defaults",
}

export enum OperationStatusEnum {
  Queued = "queued",
  Processing = "processing",
  Done = "done",
  Failed = "failed",
  Cancelled = "cancelled",
}

export enum OperationItemStatusEnum {
  Queued = "queued",
  Processing = "processing",
  Updated = "updated",
  Skipped = "skipped",
  Failed = "failed",
}

export interface Operation {
  id: string,
  userId: string,
  type: OperationTypeEnum,
  status: OperationStatusEnum,
  payload: string,
  archived: number,
  lastOperationAt: number,
  createdAt: number,
}

export interface OperationItem {
  id: string,
  operationId: string,
  title: string,
  status: OperationItemStatusEnum,
  error: string,
  position: number,
  createdAt: number,
}

// Others
export interface SelectedParameter {
  key: string
  keyName: string
  value: string
  valueName: string
}

export interface MapImporterFormItem {
  id: string
  url: string
  fileName: string
  description: string
  categories: string[]
  descriptionOverwriteBehaviour: DescriptionOverwriteBehaviour
  singleImage: boolean

  countryFileName: string
  countryDescription: string
  countryCategories: string[]
  countryDescriptionOverwriteBehaviour: DescriptionOverwriteBehaviour
  importCountries: boolean
  generateTemplateCommons: boolean
  selectedChartParameters: SelectedParameter[]
  templateNameFormat: string
  linkVerified: boolean
  templateExists: boolean
  canImport: boolean
}
