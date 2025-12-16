import { Box, Button, CircularProgress, Grid, Radio, Snackbar, Stack, TextareaAutosize, TextField, Typography } from "@mui/material";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { SocketMessage, SocketMessageTypeEnum, useWebsocket } from "../hooks/useWebsocket";
import { cancelTask, fetchTaskById, retryTask } from "../request/request";
import { DescriptionOverwriteBehaviour, Task, TaskProcess, TaskProcessStatusEnum, TaskStatusEnum, TaskTypeEnum } from "../types";
import { copyText, getStatusColor, getTaskProcessStatusColor } from "../utils";


const url_placeholder = `https://ourworldindata.org/grapher/<NAME OF GRAPH>`;

const initial_description_chart = `=={{int:filedesc}}==
{{Information
|description={{en|1=$TITLE, $REGION}}
|author = Our World In Data
|date= $END_YEAR
|source = $URL
|permission = "License: All of Our World in Data is completely open access and all work is licensed under the Creative Commons BY license. You have the permission to use, distribute, and reproduce in any medium, provided the source and authors are credited."
|other versions =
}}
=={{int:license-header}}==
{{cc-by-4.0}}
[[Category:Uploaded by OWID importer tool]]
`;
const initial_filename_chart = `$NAME, $START_YEAR to $END_YEAR, $REGION.svg`;
const chart_info_chart = `You can use $NAME (filename without extension), $START_YEAR, $END_YEAR, $REGION, $TITLE (Title of graph), and $URL as placeholders`;

const DESCRIPTION_OVERWRITE_OPTIONS = [
  {
    value: DescriptionOverwriteBehaviour.ALL,
    title: "Overwrite full description",
    description: "Overwrite the full description of the file (if already exists) with the new description.",
  },
  {
    value: DescriptionOverwriteBehaviour.ALL_EXCEPT_CATEGORIES,
    title: "Overwrite description except the categories",
    description: "Old categories are retained, any new categories in the new description are discarded/skipped. If the file doesn't already exist, categories in the new description are added.",
  },
  {
    value: DescriptionOverwriteBehaviour.ONLY_FILE,
    title: "Only upload file",
    description: "Don't update the description, only upload the file.",
  }
]

export interface ChartImporterSubmitData {
  url: string,
  fileName: string,
  description: string,
}

export interface ChartImporterProps {
  taskId?: string
}

export function ChartImporter(data: ChartImporterProps) {
  const [loading, setLoading] = useState(false);
  const [isCopied, setIsCopied] = useState(false);

  // Form Fields
  const [url, setUrl] = useState("");
  const [fileName, setFileName] = useState(initial_filename_chart);
  const [description, setDescription] = useState(initial_description_chart);
  const [descriptionOverwriteBehaviour, setDescriptionOverwriteBehaviour] = useState(DescriptionOverwriteBehaviour.ALL);

  const { ws, connect, disconnect } = useWebsocket();
  const [taskId, setTaskId] = useState("");
  const [task, setTask] = useState<Task | null>(null)
  const [items, setItems] = useState<TaskProcess[]>([]);
  const formContainerRef = useRef<HTMLDivElement>(null);
  const [maxHeight, setMaxHeight] = useState("100%");
  const [retryLoading, setRetryLoading] = useState(false);
  const [cancelLoading, setCancelLoading] = useState(false);
  const [wikiText, setWikiText] = useState("");

  const disabled = useMemo(() => {
    return !!taskId || !!data.taskId || !!task
  }, [taskId, data.taskId, task])

  const cancelDisabled = useMemo(() => {
    return !task || (task && task.status !== TaskStatusEnum.Processing)
  }, [task])

  const onRetry = () => {
    if (task) {
      setRetryLoading(true);
      retryTask(task.id)
        .then((res) => {
          console.log("retry response", res)
        })
        .catch((err) => {
          console.log("Retry error", err)
        })
        .finally(() => {
          setRetryLoading(false)
        })
    }
  }


  const getTask = useCallback((taskId: string, updateItems?: boolean) => {
    fetchTaskById(taskId)
      .then(res => {
        setFileName(res.task.filename);
        setDescription(res.task.description)
        setUrl(res.task.url);
        setDescriptionOverwriteBehaviour(res.task.descriptionOverwriteBehaviour)
        setTask(res.task);

        if (updateItems) {
          setItems(res.processes);
        }

        if (res.wikiText) {
          setWikiText(res.wikiText)
        }
      })
      .catch((err) => {
        console.log("Error fetching task", err);
      })
      .finally(() => {
        setLoading(false)
      })
  }, [setLoading, setItems, setDescription, setDescriptionOverwriteBehaviour, setUrl, setFileName, setWikiText])

  const onCancel = useCallback(() => {
    if (task) {
      setCancelLoading(true);
      cancelTask(task.id)
        .then((res) => {
          console.log("Cancel response", res)
          getTask(task.id)
        })
        .catch((err) => {
          console.log("Cancel error", err)
        })
        .finally(() => {
          setCancelLoading(false)
        })
    }
  }, [task, setCancelLoading, getTask])

  const submit = useCallback(async () => {
    setLoading(true);
    try {
      // const response = await createTask({
      //   url,
      //   fileName,
      //   description,
      //   action: "startChart",
      //   descriptionOverwriteBehaviour,
      // })
      // if (response.error) {
      //   return alert(response.error);
      // }
      // if (response.taskId) {
      //   setTaskId(response.taskId);
      // }
    } catch (err: any) {
      console.log('Error seding create task', err);
    }
    setLoading(false)
  }, [url, fileName, description, descriptionOverwriteBehaviour, setTaskId,])

  const onCopy = useCallback(() => {
    copyText(wikiText);
    setIsCopied(true);
  }, [wikiText, setIsCopied]);

  const submitDisabled = useMemo(() => {
    return !url.length || !fileName || !description;
  }, [url, fileName, description])

  const canRetry = useMemo(() => {
    if (!task) return false;
    if (task.status === TaskStatusEnum.Failed) return true;
    if (task.status === TaskStatusEnum.Done && items.some(item => item.status === TaskProcessStatusEnum.Failed)) {
      return true
    }
    return false;
  }, [task, items])

  const failedItemsCount = useMemo(() => {
    return items.filter(item => item.status === TaskProcessStatusEnum.Failed).length;
  }, [items])

  useEffect(() => {
    connect()
    return () => {
      disconnect();
    }
  }, [connect, disconnect])


  useEffect(() => {
    if (ws) {
      function listener(event: MessageEvent<any>) {
        const info = JSON.parse(event.data) as SocketMessage;
        console.log("Got websocket message: ", info)
        switch (info.type) {
          case SocketMessageTypeEnum.TASK_PROCESS:
            setItems((items) => {
              const taskProcess = JSON.parse(info.msg) as TaskProcess;
              const newItems = items.slice();
              const index = newItems.findIndex(item => item.id == taskProcess.id);
              if (index != -1) {
                newItems[index] = taskProcess;
              } else {
                newItems.splice(0, 0, taskProcess)
              }

              return newItems;
            })
            break;
          case SocketMessageTypeEnum.TASK:
            getTask((JSON.parse(info.msg) as Task).id);
            break;

        }
      }

      ws.addEventListener("message", listener);
      return () => {
        ws.removeEventListener("message", listener)
      }
    }
  }, [ws, setItems, getTask])

  useEffect(() => {
    if (ws && taskId) {
      ws.send(JSON.stringify({
        action: "subscribe_task",
        content: taskId
      }))
    }
  }, [ws, taskId])

  useEffect(() => {
    if (formContainerRef.current) {
      setMaxHeight(formContainerRef.current.getBoundingClientRect().height + "px");
    }
  }, [formContainerRef])


  useEffect(() => {
    if (taskId) {
      getTask(taskId, true);
    }
  }, [taskId, getTask])

  useEffect(() => {
    if (data.taskId) {
      setTaskId(data.taskId);
    }
  }, [data.taskId])


  return (
    <Box textAlign={"left"}>
      <Snackbar
        open={isCopied}
        autoHideDuration={2000}
        onClose={() => setIsCopied(false)}
        message="Copied succesfully"
      />
      <Grid container columnSpacing={2}>
        <Grid size={6} ref={formContainerRef}>
          <Stack sx={{ textAlign: "left" }} spacing={4}>
            <Stack spacing={2}>
              <Typography variant="h4">
                <span>Import OWID Country Chart</span>
              </Typography>
              <Typography>
                <span dangerouslySetInnerHTML={{ __html: chart_info_chart }} />
              </Typography>
            </Stack>
            <Stack spacing={2}>
              {task && (
                <Stack direction={"row"} justifyContent={"space-between"}>
                  <Stack spacing={1} direction={"row"} alignItems={"center"} textTransform={"capitalize"}>
                    <Typography >Status:</Typography>
                    <span style={{ color: getStatusColor(task.status), }}  >{task.status}</span>
                    {task.status === TaskStatusEnum.Processing && (
                      <CircularProgress size={12} color="primary" />
                    )}
                  </Stack>
                  {canRetry && (
                    <Stack>
                      <Button variant="outlined" loading={retryLoading} onClick={onRetry}>Retry failed items</Button>
                      {failedItemsCount > 0 && (
                        <Typography color="error">{failedItemsCount} Failed items</Typography>
                      )}
                    </Stack>
                  )}
                </Stack>
              )}
              <Stack spacing={1}>
                <Typography>File URL</Typography>
                <TextField
                  fullWidth
                  size="small"
                  value={url}
                  onChange={e => setUrl(e.target.value)}
                  placeholder={url_placeholder}
                  disabled={disabled}
                />
              </Stack>
              <Stack spacing={1}>
                <Typography>File name</Typography>
                <TextField
                  size="small"
                  value={fileName}
                  onChange={e => setFileName(e.target.value)}
                  fullWidth
                  disabled={disabled}
                />
              </Stack>
              <Stack spacing={1}>
                <Typography>Description</Typography>
                <TextareaAutosize
                  value={description}
                  onChange={e => setDescription(e.target.value)}
                  style={{ width: "100%", backgroundColor: "white", color: "black" }}
                  minRows={5}
                  disabled={disabled}
                />
              </Stack>
            </Stack>
            <Stack spacing={1}>
              <Typography>
                If a file with the same name exists:
              </Typography>
              {DESCRIPTION_OVERWRITE_OPTIONS.map(option => (
                <Stack spacing={1}>
                  <Stack direction={"row"} alignItems={"flex-start"}>
                    <Radio disabled={disabled} checked={descriptionOverwriteBehaviour == option.value} onClick={() => setDescriptionOverwriteBehaviour(option.value)} />
                    <Box>
                      <Typography>
                        {option.title}
                      </Typography>
                      <Typography variant="subtitle2">{option.description}</Typography>
                    </Box>
                  </Stack>
                </Stack>
              ))}
            </Stack>
            <Stack alignItems={"end"}>
              <Box>
                <Button
                  variant="contained"
                  color="primary"
                  sx={{ marginRight: 2 }}
                  onClick={submit}
                  disabled={submitDisabled || loading || disabled}
                  loading={loading}
                >
                  Submit
                </Button>
                <Button
                  onClick={onCancel}
                  disabled={cancelLoading || cancelDisabled}
                  loading={cancelLoading}
                >
                  Cancel
                </Button>
              </Box>
            </Stack>
          </Stack>
          {task && wikiText && (
            <Box marginTop={4}>
              <Stack direction="row" justifyContent="space-between" alignItems={"center"}>
                {task.type === TaskTypeEnum.MAP ? (
                  <Typography>
                    If using this with {`{{owidslider}}`}, you can use the following
                    wikicode for the gallery list page:
                  </Typography>
                ) : <Typography>
                  If using this with {`{{owidslider}}`}
                  Please add the following to your {`{{owidslidersrcs}}`}
                </Typography>
                }
                <Button onClick={onCopy}>Copy</Button>
              </Stack>
              <Box>
                <pre style={{
                  border: "1px dashed blue",
                  padding: "1em",
                }}>
                  {wikiText}
                </pre>
              </Box>
            </Box>
          )}
        </Grid>
        <Grid size={6} sx={{ maxHeight: maxHeight, overflowY: "auto" }}>
          <Stack sx={{ textAlign: "left" }}>
            {items.map(msg => (
              <Typography key={msg.id} variant="caption" color={getTaskProcessStatusColor(msg.status)}>
                {msg.region} {msg.year || ""} - <span style={{ textTransform: "capitalize" }}>{msg.status?.replace("_", " ")}</span>
                {msg.filename && (
                  <a
                    target="_blank"
                    // href={`https://commons.wikimedia.org/wiki/File:${msg.filename}`}
                    href={`http://localhost:8083/index.php/File:${msg.filename}`}
                    style={{ marginLeft: 5, textDecoration: 'underline' }}
                  >
                    Link
                  </a>
                )}
              </Typography>
            ))}
          </Stack>
        </Grid>
      </Grid>
    </Box>
  )
}

