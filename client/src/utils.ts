import { TaskProcessStatusEnum, TaskStatusEnum } from "./types";

export function getStatusColor(status: TaskStatusEnum) {
  switch (status) {
    case TaskStatusEnum.Processing:
      return "blue"
    case TaskStatusEnum.Failed:
      return "red"
    case TaskStatusEnum.Retrying:
      return "orange"
    case TaskStatusEnum.Queued:
      return "black"
    case TaskStatusEnum.Done:
      return "green"
    default:
      return "black"
  }
}

export function getTaskProcessStatusColor(status: TaskProcessStatusEnum) {
  switch (status) {
    case TaskProcessStatusEnum.Processing:
      return "blue"
    case TaskProcessStatusEnum.Failed:
      return "red"
    case TaskProcessStatusEnum.Retrying:
      return "orange"
    case TaskProcessStatusEnum.Overwritten:
    case TaskProcessStatusEnum.Uploaded:
    case TaskProcessStatusEnum.Skipped:
      return "green"
    default:
      return "black"
  }
}

export function formatDate(date: Date) {
  // Get date components
  const day = String(date.getDate()).padStart(2, '0');
  const month = String(date.getMonth() + 1).padStart(2, '0'); // getMonth() returns 0-11
  const year = date.getFullYear();

  // Get time components
  let hours = date.getHours();
  const minutes = String(date.getMinutes()).padStart(2, '0');
  const ampm = hours >= 12 ? 'pm' : 'am';

  // Convert to 12-hour format
  hours = hours % 12;
  hours = hours ? hours : 12; // If hour is 0, set it to 12
  const hoursStr = String(hours).padStart(2, '0');

  return `${month}/${day}/${year} ${hoursStr}:${minutes} ${ampm.toUpperCase()}`;
}
