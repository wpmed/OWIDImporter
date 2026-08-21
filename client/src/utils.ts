import { OperationItemStatusEnum, OperationStatusEnum, TaskProcessStatusEnum, TaskStatusEnum } from "./types";
import { StatusKind } from "./theme";

export function getStatusKind(status: TaskStatusEnum): StatusKind {
  switch (status) {
    case TaskStatusEnum.Processing:
      return "processing"
    case TaskStatusEnum.Failed:
      return "failed"
    case TaskStatusEnum.Cancelled:
      return "cancelled"
    case TaskStatusEnum.Retrying:
      return "retrying"
    case TaskStatusEnum.Done:
      return "done"
    case TaskStatusEnum.Queued:
    default:
      return "queued"
  }
}

export function getTaskProcessStatusKind(status: TaskProcessStatusEnum): StatusKind {
  switch (status) {
    case TaskProcessStatusEnum.Processing:
      return "processing"
    case TaskProcessStatusEnum.Failed:
      return "failed"
    case TaskProcessStatusEnum.Retrying:
      return "retrying"
    case TaskProcessStatusEnum.Overwritten:
    case TaskProcessStatusEnum.Uploaded:
    case TaskProcessStatusEnum.Skipped:
    case TaskProcessStatusEnum.DescriptionUpdated:
      return "done"
    default:
      return "queued"
  }
}

export function getOperationStatusKind(status: OperationStatusEnum): StatusKind {
  switch (status) {
    case OperationStatusEnum.Processing:
      return "processing"
    case OperationStatusEnum.Failed:
      return "failed"
    case OperationStatusEnum.Cancelled:
      return "cancelled"
    case OperationStatusEnum.Done:
      return "done"
    case OperationStatusEnum.Queued:
    default:
      return "queued"
  }
}

export function getOperationItemStatusKind(status: OperationItemStatusEnum): StatusKind {
  switch (status) {
    case OperationItemStatusEnum.Processing:
      return "processing"
    case OperationItemStatusEnum.Failed:
      return "failed"
    case OperationItemStatusEnum.Updated:
    case OperationItemStatusEnum.Skipped:
      return "done"
    default:
      return "queued"
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

export function copyText(text: string) {
  if (navigator.clipboard) {
    return navigator.clipboard
      .writeText(text)
      .then(function () { })
      .catch(function (err) {
        console.log("Error copying");
        console.log(err);
        copyExecCommand(text);
      });
  }

  copyExecCommand(text);
}

function copyExecCommand(text: string) {
  const span = document.createElement("span");
  span.textContent = text;

  // Preserve consecutive spaces and newlines
  span.style.whiteSpace = "pre";
  span.style.webkitUserSelect = "auto";
  span.style.userSelect = "all";

  // Add the <span> to the page
  document.body.appendChild(span);
  const selection = window.getSelection();
  const range = window.document.createRange();

  if (selection) {
    selection.removeAllRanges();
    range.selectNode(span);
    selection.addRange(range);

    // Copy text to the clipboard
    let success = false;
    try {
      success = window.document.execCommand("copy");
    } finally {
      // Cleanup
      selection.removeAllRanges();
      window.document.body.removeChild(span);
    }
    return success;
  }

  return false;
}

export function applyChartSourceToDescription(description: string, source?: string) {
  if (!source) {
    return description;
  }
  return description.replace(/author\s*=\s*Our World In Data/i, `author = ${source}`);
}

export function extractAndReplaceCategoriesFromDescription(description: string) {
  const matches = [...description.matchAll(/\[\[Category:([^\]]+)\]\]/g)];
  const categories: string[] = [];

  matches.forEach(match => {
    description = description.replace(match[0], "");
    categories.push(match[1]);
  })

  description = description.trim()
  return {
    description,
    categories
  }
}
