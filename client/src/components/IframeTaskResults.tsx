import { Box, Button, Link, Paper, Stack, Typography } from "@mui/material";
import { ContentCopy, HourglassEmpty } from "@mui/icons-material";
import { useCallback, useEffect, useMemo, useState } from "react";
import { SocketMessage, SocketMessageActionEnum, SocketMessageTypeEnum, useWebsocket } from "../hooks/useWebsocket";
import { useToast } from "../hooks/useToast";
import { cancelTask, fetchTaskById, retryTask } from "../request/request";
import { Task, TaskProcess, TaskProcessStatusEnum, TaskStatusEnum } from "../types";
import { copyText, getStatusKind, getTaskProcessStatusKind } from "../utils";
import { iframeChartUrl, IframeSubmitFailure, joinWikitextBlocks } from "../iframe";
import { monoStack } from "../theme";
import { StatusChip } from "./ui/StatusChip";
import { CodeBlock } from "./ui/CodeBlock";
import { EmptyState } from "./ui/EmptyState";

interface TaskState {
  task: Task | null
  processes: TaskProcess[]
  wikiText: string
  error: boolean
}

const EMPTY_STATE: TaskState = { task: null, processes: [], wikiText: "", error: false };

export interface IframeTaskResultsProps {
  taskIds: string[]
  // Items whose task could not be created on submit
  failedSubmissions?: IframeSubmitFailure[]
  resubmitting?: boolean
  onResubmit?: () => void
}

function isFailed(state: TaskState) {
  return state.task?.status === TaskStatusEnum.Failed || state.processes.some(p => p.status === TaskProcessStatusEnum.Failed);
}

function canRetry(state: TaskState) {
  if (!state.task) return false;
  if ([TaskStatusEnum.Failed, TaskStatusEnum.Cancelled].includes(state.task.status)) return true;
  return state.task.status === TaskStatusEnum.Done && state.processes.some(p => p.status === TaskProcessStatusEnum.Failed);
}

const NO_FAILED_SUBMISSIONS: IframeSubmitFailure[] = [];

export function IframeTaskResults({ taskIds, failedSubmissions = NO_FAILED_SUBMISSIONS, resubmitting = false, onResubmit }: IframeTaskResultsProps) {
  const { showToast } = useToast();
  const { ws, connect, disconnect } = useWebsocket();
  const [states, setStates] = useState<Record<string, TaskState>>({});
  const [busy, setBusy] = useState<Record<string, boolean>>({});

  const patch = useCallback((id: string, update: (old: TaskState) => TaskState) => {
    setStates(old => ({ ...old, [id]: update(old[id] ?? EMPTY_STATE) }));
  }, []);

  const loadTask = useCallback((id: string, updateProcesses: boolean) => {
    fetchTaskById(id)
      .then(res => {
        if (!res.task) throw new Error("task missing");
        patch(id, old => ({
          task: res.task,
          processes: updateProcesses ? (res.processes ?? []) : old.processes,
          wikiText: res.wikiText ?? "",
          error: false,
        }));
      })
      .catch(err => {
        console.log("Error fetching task", id, err);
        patch(id, old => ({ ...old, error: true }));
        showToast("Failed to load the task", "error");
      })
  }, [patch, showToast])

  const idsKey = taskIds.join(",");

  useEffect(() => {
    taskIds.forEach(id => loadTask(id, true));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [idsKey, loadTask])

  useEffect(() => {
    connect()
    return () => {
      disconnect();
    }
  }, [connect, disconnect])

  useEffect(() => {
    if (!ws) return;
    function listener(event: MessageEvent<string>) {
      const info = JSON.parse(event.data) as SocketMessage;
      if (info.type === SocketMessageTypeEnum.TASK_PROCESS) {
        const process = JSON.parse(info.msg) as TaskProcess;
        if (!taskIds.includes(process.taskId)) return;
        patch(process.taskId, old => {
          const processes = old.processes.slice();
          const index = processes.findIndex(p => p.id === process.id);
          if (index !== -1) {
            processes[index] = process;
          } else {
            processes.unshift(process);
          }
          return { ...old, processes };
        });
      } else if (info.type === SocketMessageTypeEnum.TASK) {
        const task = JSON.parse(info.msg) as Task;
        if (taskIds.includes(task.id)) {
          loadTask(task.id, false);
        }
      }
    }
    ws.addEventListener("message", listener);
    return () => {
      ws.removeEventListener("message", listener);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ws, idsKey, patch, loadTask])

  useEffect(() => {
    if (!ws) return;
    taskIds.forEach(id => ws.send(JSON.stringify({ action: SocketMessageActionEnum.SUBSCRIBE_TASK, content: id })));
    return () => {
      taskIds.forEach(id => ws.send(JSON.stringify({ action: SocketMessageActionEnum.UNSUBSCRIBE_TASK, content: id })));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ws, idsKey])

  const runAction = useCallback(async (id: string, action: "retry" | "cancel") => {
    setBusy(old => ({ ...old, [id]: true }));
    try {
      if (action === "retry") {
        await retryTask(id);
        patch(id, old => old.task ? { ...old, task: { ...old.task, status: TaskStatusEnum.Queued }, wikiText: "" } : old);
      } else {
        await cancelTask(id);
        loadTask(id, true);
      }
    } catch (err) {
      console.log(`${action} error`, err);
      showToast(`Failed to ${action} the task`, "error");
    } finally {
      setBusy(old => ({ ...old, [id]: false }));
    }
  }, [patch, loadTask, showToast])

  const ordered = taskIds.map(id => ({ id, state: states[id] ?? EMPTY_STATE }));

  const retryable = ordered.filter(({ state }) => canRetry(state));
  const failedCount = ordered.filter(({ state }) => isFailed(state)).length + failedSubmissions.length;
  const totalCount = taskIds.length + failedSubmissions.length;
  const doneCount = ordered.filter(({ state }) => state.task?.status === TaskStatusEnum.Done && !isFailed(state)).length;
  const allWikiText = useMemo(
    () => joinWikitextBlocks(ordered.map(({ state }) => state.wikiText)),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [states, idsKey]
  );

  const onCopyAll = () => {
    copyText(allWikiText);
    showToast("Copied successfully");
  }

  const onRetryAll = () => {
    retryable.forEach(({ id }) => runAction(id, "retry"));
  }

  return (
    <Stack spacing={2}>
      <Stack
        direction="row"
        justifyContent="space-between"
        alignItems="center"
        spacing={2}
        useFlexGap
        sx={{ flexWrap: 'wrap' }}
      >
        <Typography variant="body2" sx={{ color: 'text.secondary', fontFamily: monoStack }}>
          {doneCount}/{totalCount} done{failedCount > 0 ? ` · ${failedCount} failed` : ""}
        </Typography>
        <Stack direction="row" spacing={1}>
          {failedSubmissions.length > 0 && onResubmit && (
            <Button variant="outlined" color="warning" loading={resubmitting} onClick={onResubmit}>
              Resubmit {failedSubmissions.length === 1 ? "failed item" : `${failedSubmissions.length} failed items`}
            </Button>
          )}
          {taskIds.length > 1 && retryable.length > 0 && (
            <Button variant="outlined" color="warning" onClick={onRetryAll}>
              Retry failed items
            </Button>
          )}
          {taskIds.length > 1 && (
            <Button variant="outlined" startIcon={<ContentCopy />} disabled={!allWikiText} onClick={onCopyAll}>
              Copy all
            </Button>
          )}
        </Stack>
      </Stack>

      {ordered.map(({ id, state }) => {
        const { task, processes, wikiText } = state;
        const process = processes[0];
        const title = task ? (task.chartName || task.url) : "Loading...";
        const cancellable = !!task && [TaskStatusEnum.Processing, TaskStatusEnum.Queued].includes(task.status);
        return (
          <Paper key={id} variant="outlined" sx={{ p: 2 }} data-testid="iframe-task">
            <Stack spacing={1.5}>
              <Stack direction="row" justifyContent="space-between" alignItems="flex-start" spacing={2}>
                <Stack spacing={0.5} sx={{ minWidth: 0 }}>
                  <Typography sx={{ fontWeight: 600, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {title}
                  </Typography>
                  {task && (
                    <Link
                      href={iframeChartUrl(task.url)}
                      target="_blank"
                      underline="hover"
                      sx={{ fontFamily: monoStack, fontSize: 13, overflowWrap: 'anywhere' }}
                    >
                      {task.url}
                    </Link>
                  )}
                </Stack>
                {task && (
                  <StatusChip
                    kind={getStatusKind(task.status)}
                    label={task.status}
                    showSpinner={task.status === TaskStatusEnum.Processing}
                  />
                )}
              </Stack>

              {state.error && !task && (
                <Typography variant="body2" color="error">Failed to load this task.</Typography>
              )}

              {task && (
                process ? (
                  <Stack direction="row" spacing={1.5} alignItems="flex-start">
                    <Box
                      sx={{
                        width: 8,
                        height: 8,
                        mt: '6px',
                        borderRadius: '50%',
                        flexShrink: 0,
                        backgroundColor: (t) => t.palette.status[getTaskProcessStatusKind(process.status)],
                      }}
                    />
                    <Box sx={{ minWidth: 0 }}>
                      <Typography
                        variant="body2"
                        sx={{
                          textTransform: 'capitalize',
                          fontWeight: 500,
                          color: (t) => t.palette.status[getTaskProcessStatusKind(process.status)],
                        }}
                      >
                        {process.status?.replace("_", " ")}
                      </Typography>
                      {process.filename && (
                        <Link
                          target="_blank"
                          href={`${import.meta.env.VITE_MW_BASE_URL}/File:${process.filename}`}
                          sx={{ fontFamily: monoStack, fontSize: 12, overflowWrap: 'anywhere' }}
                        >
                          {process.filename}
                        </Link>
                      )}
                    </Box>
                  </Stack>
                ) : (
                  <Stack direction="row" spacing={1} alignItems="center" sx={{ color: 'text.secondary' }}>
                    <HourglassEmpty fontSize="small" />
                    <Typography variant="body2">Waiting for progress</Typography>
                  </Stack>
                )
              )}

              {task && (cancellable || canRetry(state)) && (
                <Stack direction="row" spacing={1}>
                  {canRetry(state) && (
                    <Button size="small" variant="outlined" color="warning" loading={!!busy[id]} onClick={() => runAction(id, "retry")}>
                      Retry
                    </Button>
                  )}
                  {cancellable && (
                    <Button size="small" loading={!!busy[id]} onClick={() => runAction(id, "cancel")}>
                      Cancel
                    </Button>
                  )}
                </Stack>
              )}

              {wikiText && (
                <Stack spacing={1}>
                  <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                    Wikitext for the {`{{OWID}}`} template:
                  </Typography>
                  <CodeBlock code={wikiText} />
                </Stack>
              )}
            </Stack>
          </Paper>
        )
      })}

      {failedSubmissions.map(failure => (
        <Paper key={failure.id} variant="outlined" sx={{ p: 2 }} data-testid="iframe-submit-failure">
          <Stack spacing={1}>
            <Stack direction="row" justifyContent="space-between" alignItems="flex-start" spacing={2}>
              <Typography sx={{ fontFamily: monoStack, fontSize: 13, overflowWrap: 'anywhere' }}>
                {failure.url}
              </Typography>
              <StatusChip kind="failed" label="Not submitted" />
            </Stack>
            <Typography variant="body2" color="error">{failure.error}</Typography>
          </Stack>
        </Paper>
      ))}

      {totalCount === 0 && (
        <EmptyState icon={<HourglassEmpty />} title="Nothing yet" description="Progress will appear here once the import starts." />
      )}
    </Stack>
  )
}
