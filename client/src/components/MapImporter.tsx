import { Accordion, AccordionDetails, AccordionSummary, Box, Button, Card, CardContent, CircularProgress, Grid, Snackbar, Stack, Typography } from "@mui/material";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { SocketMessage, SocketMessageTypeEnum, useWebsocket } from "../hooks/useWebsocket";
import { cancelTask, createTask, fetchTaskById, retryTask } from "../request/request";
import { DescriptionOverwriteBehaviour, MapImporterFormItem, Task, TaskProcess, TaskProcessStatusEnum, TaskStatusEnum, TaskTypeEnum } from "../types";
import { copyText, extractAndReplaceCategoriesFromDescription, getStatusColor, getTaskProcessStatusColor } from "../utils";
import { MapImporterForm } from "./MapImporterForm";
import { Add, ExpandMore, Close } from "@mui/icons-material";
import { MultiImportModal } from "./MultiImportModal";
import { generateBlankImport } from "../constants";


export interface MapImporterSubmitData {
  url: string,
  fileName: string,
  description: string,
}

export interface MapImporterProps {
  taskId?: string
  onNavigateToList: () => void
}

export function MapImporter({ taskId: incomingTaskId, onNavigateToList }: MapImporterProps) {
  const [loading, setLoading] = useState(false);
  const [parametersLoading, setParametersLoading] = useState(false);
  const [isCopied, setIsCopied] = useState(false);
  const [imports, setImports] = useState([generateBlankImport()]);
  const [expanded, setExpanded] = useState<string | false>(imports[0].id);


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
    return !!taskId || !!incomingTaskId || !!task
  }, [taskId, incomingTaskId, task])

  const cancelDisabled = useMemo(() => {
    return !task || (task && ![TaskStatusEnum.Processing, TaskStatusEnum.Queued].includes(task.status))
  }, [task])

  const onRetry = () => {
    if (task) {
      setRetryLoading(true);
      retryTask(task.id)
        .then(() => {
          setTask((task) => task ? { ...task, status: TaskStatusEnum.Queued } : null)
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
        const task = res.task;
        setTask(task);

        const { description, categories } = extractAndReplaceCategoriesFromDescription(res.task.description)
        const importItem: MapImporterFormItem = {
          ...generateBlankImport(),
          description,
          categories,
          url: task.url,
          fileName: task.filename,
          descriptionOverwriteBehaviour: task.descriptionOverwriteBehaviour,
          templateNameFormat: task.commonsTemplateNameFormat || "",
        };

        if (res.task.importCountries && res.task.countryDescription) {
          const { description, categories } = extractAndReplaceCategoriesFromDescription(res.task.countryDescription)
          importItem.countryDescription = description || "";
          importItem.countryCategories = categories;
          importItem.importCountries = !!task.importCountries;
          importItem.countryFileName = task.countryFileName || "";
          importItem.countryDescriptionOverwriteBehaviour = task.countryDescriptionOverwriteBehaviour || DescriptionOverwriteBehaviour.ALL;
        }
        setImports([importItem])

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
  }, [setLoading, setItems])

  const onMapFormChange = useCallback((index: number) => (value: MapImporterFormItem) => setImports(oldImports => {
    const newImports = oldImports.slice()
    newImports[index] = value;
    return newImports;
  }), [setImports])

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
      let taskId = ""
      await Promise.all(imports.filter(i => i.canImport).map(async (imp) => {
        let finalDescription = imp.description.trim();
        if (imp.categories.length > 0) {
          finalDescription += `\n${imp.categories.map(category => `[[Category:${category}]]`).join("\n")}`;
        }

        let finalCountryDescription = imp.countryDescription.trim()
        if (imp.countryCategories.length > 0) {
          finalCountryDescription += `\n${imp.countryCategories.map(category => `[[Category:${category}]]`).join("\n")}`;
        }

        const chartParameters = imp.selectedChartParameters.map((val) => `${val.key}=${val.value}`).join("&");
        const response = await createTask({
          action: "startMap",
          chartParameters,
          description: finalDescription,
          countryDescription: finalCountryDescription,
          url: imp.url,
          fileName: imp.fileName,
          descriptionOverwriteBehaviour: imp.descriptionOverwriteBehaviour,
          importCountries: imp.importCountries,
          generateTemplateCommons: imp.generateTemplateCommons,
          countryFileName: imp.countryFileName,
          countryDescriptionOverwriteBehaviour: imp.countryDescriptionOverwriteBehaviour,
          templateNameFormat: imp.templateNameFormat,
        });
        if (response.error) {
          return alert(response.error);
        }
        if (response.taskId) {
          taskId = response.taskId;
        }
      }));

      if (imports.length == 1 && taskId) {
        setTaskId(taskId)
      } else {
        onNavigateToList();
      }

    } catch (err: any) {
      console.log('Error seding create task', err);
    }
    setLoading(false)
  }, [
    imports,
    setTaskId,
    setLoading,
    onNavigateToList
  ])

  const onCopy = useCallback(() => {
    copyText(wikiText);
    setIsCopied(true);
  }, [wikiText, setIsCopied]);

  const submitDisabled = useMemo(() => {
    const validImports = imports.filter(i => i.url.trim().length > 0 && i.fileName.trim().length > 0 && i.description.trim().length > 0 && i.canImport);
    return loading || parametersLoading || disabled || validImports.length == 0;
  }, [imports, loading, parametersLoading, disabled])

  const canRetry = useMemo(() => {
    if (!task) return false;
    if ([TaskStatusEnum.Failed, TaskStatusEnum.Cancelled].includes(task.status)) return true;
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
        const taskProcess = JSON.parse(info.msg) as TaskProcess;
        console.log("Got websocket message: ", info)
        switch (info.type) {
          case SocketMessageTypeEnum.TASK_PROCESS:
            setItems((items) => {
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
    if (incomingTaskId) {
      setTaskId(incomingTaskId);
    }
  }, [incomingTaskId])

  return (
    <Box textAlign={"left"}>
      <Snackbar
        open={isCopied}
        autoHideDuration={2000}
        onClose={() => setIsCopied(false)}
        message="Copied succesfully"
      />      <Grid container columnSpacing={2}>
        <Grid size={6} ref={formContainerRef}>
          <Stack sx={{ textAlign: "left" }} spacing={4}>
            <Stack spacing={2}>
              <Typography variant="h4">
                <span>OWID Importer</span>
              </Typography>
            </Stack>
            {!task && (

              <Stack spacing={2}>
                <MultiImportModal onAdd={(newImports) => setImports((oldImports) => {
                  const allImports = [...oldImports, ...newImports].filter(imp => imp.url.trim())
                  return allImports;
                })} />
              </Stack>
            )}
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
              {imports.map((i, index) => {
                const comp = <MapImporterForm
                  value={i}
                  disabled={disabled}
                  canRemove={!disabled && imports.length > 1}
                  onChange={onMapFormChange(index)}
                  onParamtersLoadingChange={(loading) => setParametersLoading(loading)}
                  onDelete={() => {
                    const newImports = imports.slice();
                    newImports.splice(index, 1);
                    setImports(newImports);
                  }}
                />

                return (
                  <Box key={i.id} >
                    {imports.length > 1 ? (
                      <Accordion expanded={expanded == i.id} onChange={(_, expanded) => setExpanded(expanded ? i.id : false)}>
                        <AccordionSummary
                          id={i.id}
                          aria-controls={i.id}
                          sx={{ backgroundColor: "#1976d2", color: "white" }}
                          expandIcon={<ExpandMore sx={{ color: "white" }} />}
                        >
                          <Stack>
                            <Typography component="span">URL: {i.url} </Typography>
                            {i.linkVerified && !i.canImport && expanded !== i.id && (
                              <Stack flexDirection="row" justifyContent="center" sx={{ mt: 1, color: "orange" }}>
                                <Close />
                                <Typography>
                                  This chart cannot be imported
                                </Typography>
                              </Stack>
                            )}
                          </Stack>
                        </AccordionSummary>
                        <AccordionDetails>
                          {comp}
                        </AccordionDetails>
                      </Accordion>
                    ) : comp}
                  </Box>
                )
              }
              )}
            </Stack>

            {!submitDisabled && (
              <Stack alignItems={"center"}>
                <Button
                  startIcon={<Add />}
                  disabled={parametersLoading}
                  variant="outlined"
                  color="primary"
                  onClick={() => {
                    const newImport = generateBlankImport();
                    setImports([...imports, newImport]);
                    setExpanded(newImport.id);
                    window.scrollTo({ left: 0, top: 0 })
                  }}
                >
                  Add another URL
                </Button>
              </Stack>
            )}

            <Stack alignItems={"end"}>
              <Box>
                <Button
                  variant="contained"
                  color="primary"
                  sx={{ marginRight: 2 }}
                  onClick={submit}
                  disabled={submitDisabled}
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
                <Stack direction="row" alignItems="center">
                  {task.commonsTemplateName && (
                    <a
                      target="_blank"
                      href={`${import.meta.env.VITE_MW_BASE_URL}/${task.commonsTemplateName}`}
                      style={{ marginLeft: 5, textDecoration: 'underline' }}
                    >
                      Uploaded template
                    </a>
                  )}
                  <Button onClick={onCopy}>Copy</Button>
                </Stack>
              </Stack>
              <Box>
                <pre style={{
                  border: "1px dashed blue",
                  padding: "1em",
                  overflow: "auto"
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
                {msg.region}: {msg.date || ""} - <span style={{ textTransform: "capitalize" }}>{msg.status?.replace("_", " ")}</span>
                {msg.filename && (
                  <a
                    target="_blank"
                    href={`${import.meta.env.VITE_MW_BASE_URL}/File:${msg.filename}`}
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

