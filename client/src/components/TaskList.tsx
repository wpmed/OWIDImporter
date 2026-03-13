import { Box, Button, ButtonGroup, Grid, Stack } from "@mui/material"
import { Task, TaskStatusEnum, TaskTypeEnum } from "../types"
import { useEffect, useMemo, useState } from "react"
import { archiveTask, deleteTask, fetchTasks, retryFailedTasks } from "../request/request"
import { TaskListItem } from "./TaskListItem"
import { Refresh } from "@mui/icons-material"

interface TaskListProps {
  taskType: TaskTypeEnum
  onTaskClick: (task: Task) => void
  onNew: () => void,
}

export function TaskList({ taskType, onTaskClick, onNew }: TaskListProps) {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [archived, setArchived] = useState(0);
  // const [page, setPage] = useState(1);

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
      .then((res) => {
        console.log("Retry res: ", res);
        fetchTasks({ taskType, archived })
          .then(res => {
            if (res.tasks) {
              setTasks(res.tasks);
            }
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
    console.log("SHould get task list");
    fetchTasks({ taskType, archived })
      .then(res => {
        if (res.tasks) {
          setTasks(res.tasks);
        }
      })
      .catch(err => {
        console.log({ err });
      })

  }, [taskType, archived])

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
          <Button onClick={() => setArchived(0)} variant={archived == 0 ? "contained" : undefined}>Recent</Button>
          <Button onClick={() => setArchived(1)} variant={archived == 1 ? "contained" : undefined}>Archived</Button>
        </ButtonGroup>
        {hasFailedTasks ? (
          <Button sx={{ marginLeft: 2 }} endIcon={<Refresh />} onClick={onRetryFailedTasks} >Retry Failed Tasks</Button>
        ) : null}
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
    </Stack>
  )
}
