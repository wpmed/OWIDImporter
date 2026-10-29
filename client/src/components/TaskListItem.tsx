import { Box, Button, Card, CardContent, Divider, Link, Stack, Typography } from "@mui/material"
import { ReactNode, useEffect, useState } from "react"
import { Task, TaskStatusEnum, TaskTypeEnum } from "../types"
import { formatDate, getStatusKind } from "../utils"
import { CopyButton } from "./CopyButton"
import { StatusChip } from "./ui/StatusChip"
import { COMMONS_TEMPLATE_PREFIX } from "../constants"
import { Archive, Unarchive } from "@mui/icons-material"
import { monoStack } from "../theme"
import { iframeChartUrl, iframeFinalFileName } from "../iframe"
import { fetchTaskById } from "../request/request"

interface TaskListItemProps {
  task: Task
  onClick: () => void
  onToggleArchive: () => void
}

function CardRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <Stack direction="row" spacing={1} alignItems="center" sx={{ minWidth: 0 }}>
      <Typography
        variant="caption"
        sx={{ color: 'text.secondary', width: 64, flexShrink: 0 }}
      >
        {label}
      </Typography>
      {children}
    </Stack>
  )
}

const monoValueSx = {
  fontFamily: monoStack,
  fontSize: 13,
  minWidth: 0,
  overflow: 'hidden',
  textOverflow: 'ellipsis',
  whiteSpace: 'nowrap',
} as const;

// Done iframe tasks keep their final file name; the lookup is shared so re-renders, remounts and page
// changes do not refetch it.
const finalFileNames = new Map<string, Promise<string | undefined>>();

function loadIframeFinalFileName(taskId: string) {
  let lookup = finalFileNames.get(taskId);
  if (!lookup) {
    lookup = fetchTaskById(taskId)
      .then(res => iframeFinalFileName(res.processes ?? []))
      .catch(err => {
        console.log("Error getting iframe task file name", err);
        return undefined;
      });
    finalFileNames.set(taskId, lookup);
    // Unknown yet (or failed): let a later render try again
    lookup.then(name => { if (!name) finalFileNames.delete(taskId) });
  }
  return lookup;
}

/** The final Commons file name of a done iframe task (its list row only carries the `$NAME.svg` template). */
function useIframeFinalFileName(task: Task) {
  const enabled = task.type === TaskTypeEnum.IFRAME && task.status === TaskStatusEnum.Done;
  const [fetched, setFetched] = useState<{ id: string, name: string } | null>(null);

  useEffect(() => {
    if (!enabled) {
      return;
    }
    let cancelled = false;
    loadIframeFinalFileName(task.id).then(name => {
      if (name && !cancelled) {
        setFetched({ id: task.id, name });
      }
    });
    return () => { cancelled = true };
  }, [enabled, task.id]);

  return enabled && fetched?.id === task.id ? fetched.name : undefined;
}

export function TaskListItem({ task, onClick, onToggleArchive }: TaskListItemProps) {
  const archived = task.archived !== 0;
  // Iframe tasks store the chart path (e.g. grapher/life-expectancy), not a full URL
  const isIframe = task.type === TaskTypeEnum.IFRAME;
  const href = isIframe ? iframeChartUrl(task.url) : task.url;
  const finalFileName = useIframeFinalFileName(task);
  const fileName = finalFileName ?? task.filename;

  return (
    <Card sx={{ cursor: "pointer", height: '100%', display: 'flex' }} onClick={() => onClick()}>
      <CardContent sx={{ display: 'flex', flexDirection: 'column', flexGrow: 1, minWidth: 0 }}>
        <Stack direction="row" justifyContent="space-between" alignItems="flex-start" spacing={1} sx={{ mb: 1.5 }}>
          <Typography
            sx={{
              fontWeight: 600,
              minWidth: 0,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {task.chartName || task.url.split("/").pop()?.split("?")[0]}
          </Typography>
          <StatusChip
            kind={getStatusKind(task.status)}
            label={task.status}
            size="small"
            showSpinner={task.status === TaskStatusEnum.Processing}
          />
        </Stack>

        <Stack spacing={0.75} sx={{ minWidth: 0 }}>
          <CardRow label="URL">
            <Box onClick={(e) => e.stopPropagation()} sx={{ display: 'flex', alignItems: 'center', minWidth: 0 }}>
              <Link href={href} target="_blank" underline="hover" sx={monoValueSx}>
                {isIframe ? task.url : task.url.split("/").pop()}
              </Link>
              <CopyButton text={href} />
            </Box>
          </CardRow>
          <CardRow label="File">
            <Typography component="span" title={fileName} sx={monoValueSx}>
              {fileName}
            </Typography>
          </CardRow>
          {task.generateTemplateCommons == 1 && task.commonsTemplateName && task.status == TaskStatusEnum.Done && (
            <CardRow label="Template">
              <Box onClick={(e) => e.stopPropagation()} sx={{ display: 'flex', alignItems: 'center', minWidth: 0 }}>
                <Link
                  href={`${import.meta.env.VITE_MW_BASE_URL}/${task.commonsTemplateName}`}
                  target="_blank"
                  underline="hover"
                  sx={monoValueSx}
                >
                  {task.commonsTemplateName}
                </Link>
                <CopyButton text={`*[[${task.commonsTemplateName}|${task.commonsTemplateName.replace(COMMONS_TEMPLATE_PREFIX + "/", "")}]]`} />
              </Box>
            </CardRow>
          )}
        </Stack>

        <Divider sx={{ mt: 'auto', pt: 1.5 }} />
        <Stack
          direction="row"
          justifyContent="space-between"
          alignItems="center"
          sx={{ pt: 1 }}
          onClick={(e) => e.stopPropagation()}
        >
          <Typography variant="caption" sx={{ color: 'text.secondary' }}>
            {formatDate(new Date(task.createdAt * 1000))}
          </Typography>
          {![TaskStatusEnum.Processing, TaskStatusEnum.Queued].includes(task.status) && (
            <Button
              startIcon={archived ? <Unarchive /> : <Archive />}
              size="small"
              onClick={onToggleArchive}
            >
              {archived ? "Unarchive" : "Archive"}
            </Button>
          )}
        </Stack>
      </CardContent>
    </Card>
  )
}
