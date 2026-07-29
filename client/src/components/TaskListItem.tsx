import { Box, Button, Card, CardContent, Divider, Link, Stack, Typography } from "@mui/material"
import { ReactNode } from "react"
import { Task, TaskStatusEnum } from "../types"
import { formatDate, getStatusKind } from "../utils"
import { CopyButton } from "./CopyButton"
import { StatusChip } from "./ui/StatusChip"
import { COMMONS_TEMPLATE_PREFIX } from "../constants"
import { Archive, Unarchive } from "@mui/icons-material"
import { monoStack } from "../theme"

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

export function TaskListItem({ task, onClick, onToggleArchive }: TaskListItemProps) {
  const archived = task.archived !== 0;

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
              <Link href={task.url} target="_blank" underline="hover" sx={monoValueSx}>
                {task.url.split("/").pop()}
              </Link>
              <CopyButton text={task.url} />
            </Box>
          </CardRow>
          <CardRow label="File">
            <Typography component="span" sx={monoValueSx}>
              {task.filename}
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
