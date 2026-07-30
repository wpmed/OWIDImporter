import { Accordion, AccordionDetails, AccordionSummary, Box, Button, Divider, Grid, Link, Paper, Stack, Typography } from "@mui/material";
import { useCallback, useEffect, useMemo, useState } from "react";
import { SocketMessage, SocketMessageActionEnum, SocketMessageTypeEnum, useWebsocket } from "../hooks/useWebsocket";
import { cancelTask, createTask, fetchTaskById, retryTask } from "../request/request";
import { DescriptionOverwriteBehaviour, MapImporterFormItem, Task, TaskProcess, TaskProcessStatusEnum, TaskStatusEnum, TaskTypeEnum } from "../types";
import { extractAndReplaceCategoriesFromDescription, getStatusKind, getTaskProcessStatusKind } from "../utils";
import { MapImporterForm } from "./MapImporterForm";
import { Add, ExpandMore, HourglassEmpty, WarningAmber } from "@mui/icons-material";
import { MultiImportModal } from "./MultiImportModal";
import { generateBlankImport } from "../constants";
import { StatusChip } from "./ui/StatusChip";
import { useToast } from "../hooks/useToast";
import { PageHeader } from "./ui/PageHeader";
import { EmptyState } from "./ui/EmptyState";
import { CodeBlock } from "./ui/CodeBlock";
import { monoStack } from "../theme";


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
  const { showToast } = useToast();
  const [imports, setImports] = useState([generateBlankImport()]);
  const [expanded, setExpanded] = useState<string | false>(imports[0].id);


  const { ws, connect, disconnect } = useWebsocket();
  const [taskId, setTaskId] = useState("");
  const [task, setTask] = useState<Task | null>(null)
  const [items, setItems] = useState<TaskProcess[]>([]);
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
          showToast("Failed to retry the task", "error")
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
        showToast("Failed to load the task", "error")
      })
      .finally(() => {
        setLoading(false)
      })
  }, [setLoading, setItems, showToast])

  const onMapFormChange = useCallback((index: number) => (value: MapImporterFormItem) => setImports(oldImports => {
    const newImports = oldImports.slice()
    newImports[index] = value;
    return newImports;
  }), [setImports])

  const onCancel = useCallback(() => {
    if (task) {
      setCancelLoading(true);
      cancelTask(task.id)
        .then(() => {
          getTask(task.id)
        })
        .catch((err) => {
          console.log("Cancel error", err)
          showToast("Failed to cancel the task", "error")
        })
        .finally(() => {
          setCancelLoading(false)
        })
    }
  }, [task, setCancelLoading, getTask, showToast])

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
          return showToast(response.error, "error");
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

    } catch (err) {
      console.log('Error sending create task', err);
      showToast("Failed to create the import task", "error");
    }
    setLoading(false)
  }, [
    imports,
    setTaskId,
    setLoading,
    onNavigateToList,
    showToast
  ])

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

  const doneItemsCount = useMemo(() => {
    return items.filter(item => getTaskProcessStatusKind(item.status) === "done").length;
  }, [items])

  useEffect(() => {
    connect()
    return () => {
      disconnect();
    }
  }, [connect, disconnect])


  useEffect(() => {
    if (ws) {
      function listener(event: MessageEvent<string>) {
        const info = JSON.parse(event.data) as SocketMessage;
        const taskProcess = JSON.parse(info.msg) as TaskProcess;
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
        action: SocketMessageActionEnum.SUBSCRIBE_TASK,
        content: taskId
      }))

      return () => {
        ws.send(JSON.stringify({
          action: SocketMessageActionEnum.UNSUBSCRIBE_TASK,
          content: taskId
        }))
      }
    }
    return () => { }
  }, [ws, taskId])


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
    <Stack spacing={3}>
      <PageHeader
        title={task ? "Map import" : "Import map"}
        subtitle={task
          ? "Live progress and details for this import"
          : "Import an Our World in Data map into Wikimedia Commons"}
        action={!task ? (
          <MultiImportModal onAdd={(newImports) => setImports((oldImports) => {
            const allImports = [...oldImports, ...newImports].filter(imp => imp.url.trim())
            return allImports;
          })} />
        ) : undefined}
      />

      <Grid container spacing={3}>
        <Grid size={{ xs: 12, md: 7 }}>
          <Stack spacing={3}>
            {task && (
              <Stack direction="row" justifyContent="space-between" alignItems="center" spacing={2}>
                <Stack spacing={1} direction="row" alignItems="center">
                  <Typography sx={{ fontWeight: 600 }}>Status</Typography>
                  <StatusChip
                    kind={getStatusKind(task.status)}
                    label={task.status}
                    showSpinner={task.status === TaskStatusEnum.Processing}
                  />
                </Stack>
                {canRetry && (
                  <Stack alignItems="flex-end" spacing={0.5}>
                    <Button variant="outlined" color="warning" loading={retryLoading} onClick={onRetry}>
                      Retry failed items
                    </Button>
                    {failedItemsCount > 0 && (
                      <Typography variant="caption" color="error">{failedItemsCount} failed items</Typography>
                    )}
                  </Stack>
                )}
              </Stack>
            )}

            <Stack spacing={2}>
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
                  <Box key={i.id}>
                    {imports.length > 1 ? (
                      <Accordion expanded={expanded == i.id} onChange={(_, expanded) => setExpanded(expanded ? i.id : false)}>
                        <AccordionSummary
                          id={i.id}
                          aria-controls={i.id}
                          expandIcon={<ExpandMore />}
                        >
                          <Stack spacing={0.5} sx={{ minWidth: 0 }}>
                            <Typography
                              component="span"
                              sx={{
                                fontFamily: monoStack,
                                fontSize: 13,
                                overflow: 'hidden',
                                textOverflow: 'ellipsis',
                                whiteSpace: 'nowrap',
                              }}
                            >
                              {i.url || "New import"}
                            </Typography>
                            {i.linkVerified && !i.canImport && expanded !== i.id && (
                              <Stack direction="row" spacing={0.5} alignItems="center" sx={{ color: 'warning.main' }}>
                                <WarningAmber fontSize="small" />
                                <Typography variant="body2">
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
              <Stack alignItems="center">
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

            <Stack direction="row" justifyContent="flex-end" spacing={2}>
              <Button
                onClick={onCancel}
                disabled={cancelLoading || cancelDisabled}
                loading={cancelLoading}
              >
                Cancel
              </Button>
              <Button
                variant="contained"
                color="primary"
                onClick={submit}
                disabled={submitDisabled}
                loading={loading}
              >
                Submit
              </Button>
            </Stack>

            {task && wikiText && (
              <Stack spacing={1.5}>
                <Stack direction="row" justifyContent="space-between" alignItems="center" spacing={2}>
                  <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                    {task.type === TaskTypeEnum.MAP ? (
                      <>If using this with {`{{owidslider}}`}, you can use the following wikicode for the gallery list page:</>
                    ) : (
                      <>If using this with {`{{owidslider}}`}, please add the following to your {`{{owidslidersrcs}}`}:</>
                    )}
                  </Typography>
                  {task.commonsTemplateName && (
                    <Link
                      target="_blank"
                      href={`${import.meta.env.VITE_MW_BASE_URL}/${task.commonsTemplateName}`}
                      sx={{ whiteSpace: 'nowrap' }}
                    >
                      Uploaded template
                    </Link>
                  )}
                </Stack>
                <CodeBlock code={wikiText} />
              </Stack>
            )}
          </Stack>
        </Grid>

        <Grid size={{ xs: 12, md: 5 }}>
          <Paper
            variant="outlined"
            sx={{
              position: { md: 'sticky' },
              top: 88,
              display: 'flex',
              flexDirection: 'column',
              maxHeight: { md: 'calc(100vh - 120px)' },
            }}
          >
            <Stack direction="row" justifyContent="space-between" alignItems="baseline" sx={{ px: 2, py: 1.5 }}>
              <Typography sx={{ fontWeight: 600 }}>Progress</Typography>
              {items.length > 0 && (
                <Typography variant="caption" sx={{ color: 'text.secondary', fontFamily: monoStack }}>
                  {doneItemsCount}/{items.length}
                  {failedItemsCount > 0 ? ` · ${failedItemsCount} failed` : ""}
                </Typography>
              )}
            </Stack>
            <Divider />
            <Box sx={{ overflowY: 'auto', flexGrow: 1 }}>
              {items.length === 0 ? (
                <Box sx={{ p: 2 }}>
                  <EmptyState
                    icon={<HourglassEmpty />}
                    title="Nothing yet"
                    description="Per-region progress will appear here once the import starts."
                  />
                </Box>
              ) : (
                items.map(msg => (
                  <Stack
                    key={msg.id}
                    direction="row"
                    spacing={1.5}
                    alignItems="flex-start"
                    sx={{ px: 2, py: 1, borderBottom: '1px solid', borderColor: 'divider' }}
                  >
                    <Box
                      sx={{
                        width: 8,
                        height: 8,
                        mt: '6px',
                        borderRadius: '50%',
                        flexShrink: 0,
                        backgroundColor: (t) => t.palette.status[getTaskProcessStatusKind(msg.status)],
                      }}
                    />
                    <Box sx={{ minWidth: 0 }}>
                      <Typography variant="body2" sx={{ fontWeight: 500 }}>
                        {msg.region}{msg.date ? ` · ${msg.date}` : ""}
                      </Typography>
                      <Typography
                        variant="caption"
                        sx={{
                          textTransform: 'capitalize',
                          color: (t) => t.palette.status[getTaskProcessStatusKind(msg.status)],
                        }}
                      >
                        {msg.status?.replace("_", " ")}
                      </Typography>
                      {msg.filename && (
                        <Link
                          target="_blank"
                          href={`${import.meta.env.VITE_MW_BASE_URL}/File:${msg.filename}`}
                          sx={{ ml: 1, fontFamily: monoStack, fontSize: 12 }}
                        >
                          {msg.filename}
                        </Link>
                      )}
                    </Box>
                  </Stack>
                ))
              )}
            </Box>
          </Paper>
        </Grid>
      </Grid>
    </Stack>
  )
}
