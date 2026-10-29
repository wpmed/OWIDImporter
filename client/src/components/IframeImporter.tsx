import { Accordion, AccordionDetails, AccordionSummary, Box, Button, Stack, Typography } from "@mui/material";
import { Add, ExpandMore, WarningAmber } from "@mui/icons-material";
import { useCallback, useMemo, useState } from "react";
import { createTask } from "../request/request";
import { IframeImporterFormItem } from "../types";
import { generateBlankIframeImport } from "../constants";
import { buildDescriptionWithCategories, IframeItemUpdate, IframeSubmitFailure } from "../iframe";
import { useToast } from "../hooks/useToast";
import { monoStack } from "../theme";
import { PageHeader } from "./ui/PageHeader";
import { IframeImporterForm } from "./IframeImporterForm";
import { IframeTaskResults } from "./IframeTaskResults";
import { MultiImportModal } from "./MultiImportModal";

export interface IframeImporterProps {
  // Opens an existing task (from the Iframe imports list)
  taskId?: string
}

export function IframeImporter({ taskId: incomingTaskId }: IframeImporterProps) {
  const { showToast } = useToast();
  const [imports, setImports] = useState([generateBlankIframeImport()]);
  const [expanded, setExpanded] = useState<string | false>(imports[0].id);
  const [loading, setLoading] = useState(false);
  const [infoLoading, setInfoLoading] = useState(false);
  const [createdTaskIds, setCreatedTaskIds] = useState<string[]>(incomingTaskId ? [incomingTaskId] : []);

  const [submitFailures, setSubmitFailures] = useState<{ item: IframeImporterFormItem, error: string }[]>([]);

  const taskIds = createdTaskIds;
  const showingTasks = taskIds.length > 0 || submitFailures.length > 0;

  // The form passes an update derived from the item as it is now, not a snapshot that may be stale
  const onFormChange = useCallback((id: string) => (update: IframeItemUpdate) => setImports(old => {
    return old.map(item => item.id === id ? update(item) : item);
  }), [])

  // Creates one task per item; Promise.all keeps the order of the cards.
  const createTasks = useCallback(async (items: IframeImporterFormItem[]) => {
    const results = await Promise.all(items.map(async (imp) => {
      try {
        const response = await createTask({
          action: "startIframe",
          url: imp.url.trim(),
          fileName: imp.fileName,
          description: buildDescriptionWithCategories(imp.description, imp.categories),
          descriptionOverwriteBehaviour: imp.descriptionOverwriteBehaviour,
        });
        if (response.error || !response.taskId) {
          return { item: imp, error: response.error || "No task was created" };
        }
        return { item: imp, taskId: response.taskId };
      } catch (err) {
        console.log("Error sending create task", err);
        return { item: imp, error: "Failed to create the import task" };
      }
    }));
    return results;
  }, [])

  const submitItems = useCallback(async (items: IframeImporterFormItem[]) => {
    setLoading(true);
    try {
      const results = await createTasks(items);
      const ids = results.flatMap(r => r.taskId ? [r.taskId] : []);
      const failed = results.flatMap(r => r.taskId ? [] : [{ item: r.item, error: r.error ?? "" }]);
      setCreatedTaskIds(old => [...old, ...ids]);
      setSubmitFailures(failed);
      if (failed.length > 0) {
        showToast(`${failed.length} of ${items.length} could not be submitted`, "error");
      }
      if (ids.length > 0 || failed.length > 0) {
        window.scrollTo({ left: 0, top: 0 });
      }
    } finally {
      setLoading(false);
    }
  }, [createTasks, showToast])

  const submit = useCallback(() => submitItems(imports.filter(i => i.canImport)), [imports, submitItems])

  const resubmit = useCallback(() => submitItems(submitFailures.map(f => f.item)), [submitFailures, submitItems])

  const failedSubmissions = useMemo<IframeSubmitFailure[]>(
    () => submitFailures.map(f => ({ id: f.item.id, url: f.item.url.trim(), error: f.error })),
    [submitFailures]
  );

  const submitDisabled = useMemo(() => {
    const validImports = imports.filter(i => i.url.trim().length > 0 && i.fileName.trim().length > 0 && i.description.trim().length > 0 && i.canImport);
    return loading || infoLoading || validImports.length == 0;
  }, [imports, loading, infoLoading])

  return (
    <Stack spacing={3}>
      <PageHeader
        title={showingTasks ? "Iframe import" : "Import iframe"}
        subtitle={showingTasks
          ? "Live progress and {{OWID}} wikitext for this import"
          : "Import an Our World in Data chart as a Wikipedia-optimized SVG and get the {{OWID}} wikitext"}
        action={!showingTasks ? (
          <MultiImportModal
            variant="iframe"
            onAddIframe={(newImports) => setImports((oldImports) => {
              return [...oldImports, ...newImports].filter(imp => imp.url.trim());
            })}
          />
        ) : undefined}
      />

      {showingTasks ? (
        <IframeTaskResults taskIds={taskIds} failedSubmissions={failedSubmissions} resubmitting={loading} onResubmit={resubmit} />
      ) : (
        <Stack spacing={3} sx={{ maxWidth: 900 }}>
          <Stack spacing={2}>
            {imports.map((i) => {
              const comp = <IframeImporterForm
                value={i}
                disabled={loading}
                canRemove={imports.length > 1}
                onChange={onFormChange(i.id)}
                onInfoLoadingChange={setInfoLoading}
                onDelete={() => setImports(old => old.filter(item => item.id !== i.id))}
              />

              return (
                <Box key={i.id}>
                  {imports.length > 1 ? (
                    <Accordion expanded={expanded == i.id} onChange={(_, isExpanded) => setExpanded(isExpanded ? i.id : false)}>
                      <AccordionSummary
                        id={i.id}
                        aria-controls={i.id}
                        expandIcon={<ExpandMore />}
                        // Lets a long URL shrink to an ellipsis instead of pushing the header past the card
                        sx={{ '& .MuiAccordionSummary-content': { minWidth: 0 } }}
                      >
                        <Stack spacing={0.5} sx={{ minWidth: 0 }}>
                          <Typography
                            component="span"
                            title={i.url || undefined}
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
            })}
          </Stack>

          {!submitDisabled && (
            <Stack alignItems="center">
              <Button
                startIcon={<Add />}
                disabled={infoLoading}
                variant="outlined"
                color="primary"
                onClick={() => {
                  const newImport = generateBlankIframeImport();
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
              variant="contained"
              color="primary"
              onClick={submit}
              disabled={submitDisabled}
              loading={loading}
            >
              Submit
            </Button>
          </Stack>
        </Stack>
      )}
    </Stack>
  )
}
