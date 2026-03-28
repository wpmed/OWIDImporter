import { Button, Card, CardContent, CircularProgress, Grid, Stack, Typography } from "@mui/material"
import { Task, TaskStatusEnum } from "../types"
import { formatDate, getStatusColor } from "../utils"
import { CopyButton } from "./CopyButton"
import { COMMONS_TEMPLATE_PREFIX } from "../constants"
import { Archive } from "@mui/icons-material"

interface TaskListItemProps {
  task: Task
  onClick: () => void
  onDelete?: () => void
  onToggleArchive: () => void
}

export function TaskListItem({ task, onClick, onToggleArchive }: TaskListItemProps) {


  const toggleArchive = () => {
    onToggleArchive()
  }



  return (

    <Card sx={{ cursor: "pointer" }} onClick={() => onClick()}>
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

        <Stack
          sx={{ mt: 1 }}
          flexDirection={"row"}
          justifyContent={"flex-end"}
          onClick={(e) => e.stopPropagation()}
        >
          {/* {task.status !== TaskStatusEnum.Processing && ( */}
          {/*   <PopoverConfirmationButton */}
          {/*     id={`delete-${task.id}`} */}
          {/*     trigger={( */}
          {/*       <Button color="error" startIcon={<Delete />} aria-describedby={`delete-${task.id}`} size="small"> */}
          {/*         Delete */}
          {/*       </Button> */}
          {/*     )} */}
          {/*     onOk={toggleArchive} */}
          {/*     okText="Yes" */}
          {/*     message={ */}
          {/*       <> */}
          {/*         Are you sure you want to delete this task? */}
          {/*       </> */}
          {/*     } */}
          {/*   /> */}
          {/* )} */}
          {![TaskStatusEnum.Processing, TaskStatusEnum.Queued].includes(task.status) && (
            <Button startIcon={<Archive />} aria-describedby={`archive-${task.id}`} size="medium" onClick={toggleArchive}>
              {task.archived == 0 ? <span>Archive</span> : <span>UnArchive</span>}
            </Button>
          )}
          {/* <PopoverConfirmationButton */}
          {/*   id={`archive-${task.id}`} */}
          {/*   trigger={( */}
          {/*     <Button startIcon={<Archive />} aria-describedby={`archive-${task.id}`} size="small"> */}
          {/*       {task.archived == 0 ? <span>Archive</span> : <span>UnArchive</span>} */}
          {/*     </Button> */}
          {/*   )} */}
          {/*   onOk={toggleArchive} */}
          {/*   okText="Yes" */}
          {/*   message={ */}
          {/*     <> */}
          {/*       Are you sure you want to {task.archived == 0 ? <span>Archive</span> : <span>UnArchive</span>} this task? */}
          {/*     </> */}
          {/*   } */}
          {/* /> */}
        </Stack>
      </CardContent>
    </Card>
  )
}
