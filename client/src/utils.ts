import { TaskProcessStatusEnum, TaskStatusEnum } from "./types";

export function getStatusColor(status: TaskStatusEnum) {
  switch (status) {
    case TaskStatusEnum.Processing:
      return "blue"
    case TaskStatusEnum.Failed:
      return "red"
    case TaskStatusEnum.Cancelled:
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
    case TaskProcessStatusEnum.DescriptionUpdated:
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
