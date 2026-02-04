import { Box, Button, Card, CardContent, CircularProgress, Grid, Stack, Typography } from "@mui/material"
import { Task, TaskStatusEnum, TaskTypeEnum } from "../types"
import { formatDate, getStatusColor } from "../utils"
import { CopyButton } from "./CopyButton"
import { COMMONS_TEMPLATE_PREFIX } from "../constants"

interface TaskListProps {
  tasks: Task[],
  taskType: TaskTypeEnum
  onTaskClick: (task: Task) => void
  onNew: () => void,
}

export function TaskList({ tasks, taskType, onTaskClick, onNew }: TaskListProps) {
  return (
    <Stack spacing={2} textAlign={"left"}>
      <Box>
        <Button sx={{ textTransform: "capitalize" }} variant="contained" onClick={onNew}>
          Import New {taskType}
        </Button>
      </Box>
      <Grid container spacing={4}>
        {tasks.map(task => (
          <Grid size={4} key={task.id}>
            <Card sx={{ cursor: "pointer" }} onClick={() => onTaskClick(task)}>
              <CardContent >
                <Stack spacing={1}>
                  {task.chartName && (
                    <Grid container spacing={1}>
                      <Grid size={3}>
                        <Typography variant="body2">
                          Chart Name:
                        </Typography>
                      </Grid>
                      <Grid>
                        <Typography variant="body2">
                          {task.chartName}
                        </Typography>
                      </Grid>
                    </Grid>
                  )}
                  <Grid container spacing={1}>
                    <Grid size={3}>
                      <Typography variant="body2">
                        URL:
                      </Typography>
                    </Grid>
                    <Grid onClick={(e) => e.stopPropagation()}>
                      <Stack justifyContent={"space-between"} alignItems={"center"} flexDirection={"row"}>
                        <Typography gutterBottom sx={{ color: 'text.secondary', fontSize: 14 }}>
                          <a href={task.url} target="_blank">
                            {task.url.split("/").pop()}
                          </a>
                        </Typography>
                        <CopyButton text={task.url} />
                      </Stack>
                    </Grid>
                  </Grid>
                  {task.generateTemplateCommons == 1 && task.commonsTemplateName && task.status == TaskStatusEnum.Done ? (
                    <Grid container spacing={1}>
                      <Grid size={3}>
                        <Typography variant="body2">
                          Commons Template:
                        </Typography>
                      </Grid>
                      <Grid onClick={(e) => e.stopPropagation()}>
                        <Stack justifyContent={"space-between"} alignItems={"center"} flexDirection={"row"}>
                          <Typography gutterBottom sx={{ color: 'text.secondary', fontSize: 14 }}>
                            <a href={`${import.meta.env.VITE_MW_BASE_URL}/${task.commonsTemplateName}`} target="_blank">
                              {task.commonsTemplateName}
                            </a>
                          </Typography>
                          <CopyButton text={`*[[${task.commonsTemplateName}|${task.commonsTemplateName.replace(COMMONS_TEMPLATE_PREFIX + "/", "")}]]`} />
                        </Stack>
                      </Grid>
                    </Grid>
                  ) : null}
                  <Grid container spacing={1}>
                    <Grid size={3}>
                      <Typography variant="body2">
                        File Name:
                      </Typography>
                    </Grid>
                    <Grid>
                      <Typography variant="body2">
                        {task.filename}
                      </Typography>
                    </Grid>
                  </Grid>
                  <Grid container spacing={1}>
                    <Grid size={3}>
                      <Typography variant="body2">
                        Status:
                      </Typography>
                    </Grid>
                    <Grid>
                      <Stack spacing={1} direction={"row"} alignItems={"center"} textTransform={"capitalize"}>
                        <span style={{ color: getStatusColor(task.status), }}  >{task.status}</span>
                        {task.status === TaskStatusEnum.Processing && (
                          <CircularProgress size={12} color="primary" />
                        )}
                      </Stack>
                    </Grid>
                  </Grid>
                  <Grid container spacing={1}>
                    <Grid size={3}>
                      <Typography variant="body2">
                        Created At:
                      </Typography>
                    </Grid>
                    <Grid>
                      <Typography variant="body2">
                        {formatDate(new Date(task.createdAt * 1000))}
                      </Typography>

                    </Grid>
                  </Grid>
                </Stack>
              </CardContent>
            </Card>
          </Grid>
        ))}
      </Grid>
    </Stack>
  )
}
