import { Box, Button, FormControl, Grid, InputAdornment, InputLabel, MenuItem, Pagination, Select, Stack, TextField, ToggleButton, ToggleButtonGroup } from "@mui/material"
import { Task, TaskStatusEnum, TaskTypeEnum } from "../types"
import { useEffect, useMemo, useRef, useState } from "react"
import { archiveTask, fetchTasks, retryFailedTasks } from "../request/request"
import { TaskListItem } from "./TaskListItem"
import { Add, Map as MapIcon, Refresh, Search, SearchOff } from "@mui/icons-material"
import { SocketMessage, SocketMessageActionEnum, useWebsocket } from "../hooks/useWebsocket"
import { useToast } from "../hooks/useToast"
import { PageHeader } from "./ui/PageHeader"
import { EmptyState } from "./ui/EmptyState"
import { TaskCardSkeleton } from "./ui/TaskCardSkeleton"

const STATUS_OPTIONS: { label: string; value: string }[] = [
  { label: "All", value: "" },
  { label: "Queued", value: TaskStatusEnum.Queued },
  { label: "Processing", value: TaskStatusEnum.Processing },
  { label: "Done", value: TaskStatusEnum.Done },
  { label: "Retrying", value: TaskStatusEnum.Retrying },
  { label: "Failed", value: TaskStatusEnum.Failed },
  { label: "Cancelled", value: TaskStatusEnum.Cancelled },
]

interface TaskListProps {
  taskType: TaskTypeEnum
  onTaskClick: (task: Task) => void
  onNew: () => void,
}

export function TaskList({ taskType, onTaskClick, onNew }: TaskListProps) {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [loading, setLoading] = useState(true);
  const [archived, setArchived] = useState(0);
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [perPage, setPerPage] = useState(20);
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("");
  const debounceTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const { ws, connect, disconnect } = useWebsocket();
  const { showToast } = useToast();

  const filtersActive = search.trim() !== "" || status !== "";

  const onToggleTaskArchived = (task: Task) => {
    const newArchived = task.archived === 0 ? 1 : 0;

    archiveTask(task.id, newArchived)
      .then((res) => {
        if (res.task) {
          setTasks((tasks) => {
            const newTasks = tasks.slice().filter(t => t.id != task.id);
            return newTasks;
          });
        }
      })
      .catch(err => {
        console.log("Error updating archive: ", newArchived, err)
        showToast("Failed to update the task", "error")
      })
  }

  const hasFailedTasks = useMemo(() => {
    return tasks.some(t => t.status == TaskStatusEnum.Failed);
  }, [tasks])

  const onRetryFailedTasks = () => {
    retryFailedTasks()
      .then(() => {
        fetchTasks({ taskType, archived, page, perPage, search, status })
          .then(res => {
            if (res.tasks) {
              setTasks(res.tasks);
            }
            setTotalPages(res.totalPages > 0 ? res.totalPages : 1);
          })
          .catch(err => {
            console.log({ err });
          })
      })
      .catch(err => {
        console.log(err);
        showToast("Failed to retry tasks", "error")
      })
  }

  useEffect(() => {
    if (debounceTimer.current) clearTimeout(debounceTimer.current);
    debounceTimer.current = setTimeout(() => {
      setSearch(searchInput);
      setPage(1);
    }, 400);
    return () => {
      if (debounceTimer.current) clearTimeout(debounceTimer.current);
    };
  }, [searchInput]);

  useEffect(() => {
    setLoading(true);
    fetchTasks({ taskType, archived, page, perPage, search, status })
      .then(res => {
        if (res.tasks) {
          setTasks(res.tasks);
        }
        setTotalPages(res.totalPages > 0 ? res.totalPages : 1);
      })
      .catch(err => {
        console.log({ err });
        showToast("Failed to load tasks", "error")
      })
      .finally(() => {
        setLoading(false);
      })

  }, [taskType, archived, page, perPage, search, status, showToast])

  useEffect(() => {
    if (ws) {
      function listener(event: MessageEvent<string>) {
        const info = JSON.parse(event.data) as SocketMessage;
        const task = JSON.parse(info.msg) as Task;
        setTasks((tasks) => {
          const newTasks = tasks.slice();
          const taskIndex = newTasks.findIndex(t => t.id === task.id)
          if (taskIndex !== -1) {
            newTasks[taskIndex] = task;
          }

          return newTasks;
        })
      }

      ws.addEventListener("message", listener);
      return () => {
        ws.removeEventListener("message", listener)
      }
    }

    return () => { }
  }, [ws, setTasks])

  useEffect(() => {
    if (ws) {
      ws.send(JSON.stringify({
        action: SocketMessageActionEnum.SUBSCRIBE_TASK_LIST,
      }))

      return () => {
        ws.send(JSON.stringify({
          action: SocketMessageActionEnum.UNSUBSCRIBE_TASK_LIST,
        }))
      }
    }
    return () => { }
  }, [ws])

  useEffect(() => {
    connect()
    return () => {
      disconnect();
    }
  }, [connect, disconnect])

  return (
    <Stack spacing={3}>
      <PageHeader
        title="Map imports"
        subtitle="Imports of Our World in Data maps into Wikimedia Commons"
        action={(
          <Button variant="contained" startIcon={<Add />} onClick={onNew}>
            New import
          </Button>
        )}
      />

      <Stack
        direction="row"
        alignItems="center"
        spacing={2}
        useFlexGap
        sx={{ flexWrap: 'wrap' }}
      >
        <ToggleButtonGroup
          exclusive
          size="small"
          value={archived}
          onChange={(_, value) => {
            if (value !== null) {
              setArchived(value);
              setPage(1);
            }
          }}
        >
          <ToggleButton value={0}>Recent</ToggleButton>
          <ToggleButton value={1}>Archived</ToggleButton>
        </ToggleButtonGroup>
        <TextField
          size="small"
          placeholder="Chart name, URL, or file name"
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
          sx={{ width: { xs: '100%', sm: 320 } }}
          slotProps={{
            input: {
              startAdornment: (
                <InputAdornment position="start">
                  <Search fontSize="small" />
                </InputAdornment>
              ),
            },
          }}
        />
        <FormControl size="small">
          <InputLabel>Status</InputLabel>
          <Select
            label="Status"
            value={status}
            onChange={(e) => { setStatus(e.target.value); setPage(1); }}
            sx={{ width: 140 }}
          >
            {STATUS_OPTIONS.map(opt => (
              <MenuItem key={opt.value} value={opt.value}>{opt.label}</MenuItem>
            ))}
          </Select>
        </FormControl>
        <FormControl size="small">
          <InputLabel>Per page</InputLabel>
          <Select
            label="Per page"
            value={perPage}
            onChange={(e) => { setPerPage(Number(e.target.value)); setPage(1); }}
            sx={{ width: 100 }}
          >
            {[2, 10, 20, 50, 100].map(n => (
              <MenuItem key={n} value={n}>{n}</MenuItem>
            ))}
          </Select>
        </FormControl>
        {hasFailedTasks && (
          <Button variant="outlined" color="warning" endIcon={<Refresh />} onClick={onRetryFailedTasks}>
            Retry failed tasks
          </Button>
        )}
      </Stack>

      {loading && tasks.length === 0 ? (
        <Grid container spacing={3}>
          {Array.from({ length: 6 }, (_, i) => (
            <Grid size={{ xs: 12, sm: 6, lg: 4 }} key={i}>
              <TaskCardSkeleton />
            </Grid>
          ))}
        </Grid>
      ) : tasks.length === 0 ? (
        filtersActive ? (
          <EmptyState
            icon={<SearchOff />}
            title="No matching imports"
            description="No imports match the current search or status filter. Try clearing the filters."
          />
        ) : (
          <EmptyState
            icon={<MapIcon />}
            title={archived ? "No archived imports" : "No imports yet"}
            description={archived
              ? "Imports you archive will show up here."
              : "Import your first Our World in Data map to see it here."}
            action={!archived && (
              <Button variant="contained" startIcon={<Add />} onClick={onNew}>
                New import
              </Button>
            )}
          />
        )
      ) : (
        <Grid container spacing={3}>
          {tasks.map(task => (
            <Grid size={{ xs: 12, sm: 6, lg: 4 }} key={task.id}>
              <TaskListItem
                task={task}
                onClick={() => onTaskClick(task)}
                onToggleArchive={() => onToggleTaskArchived(task)}
              />
            </Grid>
          ))}
        </Grid>
      )}

      {totalPages > 1 && (
        <Box display="flex" justifyContent="center" mt={2}>
          <Pagination
            count={totalPages}
            page={page}
            onChange={(_, value) => setPage(value)}
            color="primary"
          />
        </Box>
      )}
    </Stack>
  )
}
