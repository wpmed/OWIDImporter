import { Box, Button, ButtonGroup, FormControl, Grid, InputLabel, MenuItem, Pagination, Select, Stack, TextField } from "@mui/material"
import { Task, TaskStatusEnum, TaskTypeEnum } from "../types"
const STATUS_OPTIONS: { label: string; value: string }[] = [
  { label: "All", value: "" },
  { label: "Queued", value: TaskStatusEnum.Queued },
  { label: "Processing", value: TaskStatusEnum.Processing },
  { label: "Done", value: TaskStatusEnum.Done },
  { label: "Retrying", value: TaskStatusEnum.Retrying },
  { label: "Failed", value: TaskStatusEnum.Failed },
  { label: "Cancelled", value: TaskStatusEnum.Cancelled },
]
import { useEffect, useMemo, useRef, useState } from "react"
import { archiveTask, deleteTask, fetchTasks, retryFailedTasks } from "../request/request"
import { TaskListItem } from "./TaskListItem"
import { Refresh } from "@mui/icons-material"
import { SocketMessage, SocketMessageActionEnum, useWebsocket } from "../hooks/useWebsocket"

interface TaskListProps {
  taskType: TaskTypeEnum
  onTaskClick: (task: Task) => void
  onNew: () => void,
}

export function TaskList({ taskType, onTaskClick, onNew }: TaskListProps) {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [archived, setArchived] = useState(0);
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [perPage, setPerPage] = useState(20);
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("");
  const debounceTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const { ws, connect, disconnect } = useWebsocket();

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
      })
  }

  const onDeleteTask = (task: Task) => {
    deleteTask(task.id)
      .then((res) => {
        if (res.task) {
          setTasks((tasks) => {
            const newTasks = tasks.slice().filter(t => t.id != task.id);
            return newTasks;
          });
        }
      })
      .catch(err => {
        console.log("Error deleting task: ", err)
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

  }, [taskType, archived, page, perPage, search, status])

  useEffect(() => {
    if (ws) {
      function listener(event: MessageEvent<any>) {
        const info = JSON.parse(event.data) as SocketMessage;
        const task = JSON.parse(info.msg) as Task;
        setTasks((tasks) => {
          const newTasks = tasks.slice();
          const taskIndex = newTasks.findIndex(t => t.id === task.id)
          console.log({taskIndex})
          if (taskIndex !== -1) {
            newTasks[taskIndex] = task;
          }

          return newTasks;
        })
      }

      ws.addEventListener("message", listener);
      console.log("Added listener")
      return () => {
        ws.removeEventListener("message", listener)
        console.log("Removed listener")
      }
    }

    return () => {}
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
    <Stack spacing={2} textAlign={"left"}>
      <Box>
        <Button sx={{ textTransform: "capitalize" }} variant="contained" onClick={onNew}>
          Import New {taskType}
        </Button>
      </Box>
      <hr />
      <Box>
        <ButtonGroup>
          <Button onClick={() => { setArchived(0); setPage(1); }} variant={archived == 0 ? "contained" : undefined}>Recent</Button>
          <Button onClick={() => { setArchived(1); setPage(1); }} variant={archived == 1 ? "contained" : undefined}>Archived</Button>
        </ButtonGroup>
        {hasFailedTasks ? (
          <Button sx={{ marginLeft: 2 }} endIcon={<Refresh />} onClick={onRetryFailedTasks} >Retry Failed Tasks</Button>
        ) : null}
      </Box>

      <Box display="flex" alignItems="center" gap={2}>
        <TextField
          size="small"
          label="Search"
          placeholder="Chart name, URL, or file name"
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
          sx={{ width: 320 }}
        />
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
      </Box>

      <Grid container spacing={4}>
        {tasks.map(task => (
          <Grid size={4} key={task.id}>
            <TaskListItem
              task={task}
              onClick={() => onTaskClick(task)}
              onToggleArchive={() => onToggleTaskArchived(task)}
              onDelete={() => onDeleteTask(task)}
            />
          </Grid>
        ))}
      </Grid>

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
