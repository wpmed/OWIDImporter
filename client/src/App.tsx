import './App.css'
import Box from '@mui/material/Box';
import Drawer from '@mui/material/Drawer';
import AppBar from '@mui/material/AppBar';
import Toolbar from '@mui/material/Toolbar';
import List from '@mui/material/List';
import Typography from '@mui/material/Typography';
import ListItem from '@mui/material/ListItem';
import ListItemButton from '@mui/material/ListItemButton';
import ListItemText from '@mui/material/ListItemText';
import MapIcon from '@mui/icons-material/Map';
// import ChartIcon from '@mui/icons-material/PieChart';

import { ListItemIcon } from '@mui/material';
import { MapImporter } from './components/MapImporter';
import { Login } from './components/Login';
import { useReplaceSession } from './hooks/useReplaceSession';
import { SESSION_ID_KEY, USERNAME_KEY } from './constants';
import { Logout } from '@mui/icons-material';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { Task, TaskTypeEnum } from './types';
import { fetchTasks, logout } from './request/request';
import { TaskList } from './components/TaskList';
import { ChartImporter } from './components/ChartImporter';

const drawerWidth = 240;

enum TABS {
  MAP_LSIT = 0,
  MAP_DETAILS = 1,
  CHART_LIST = 2,
  CHART_DETAILS = 3,
}

const LIST_ITEMS = [
  {
    id: TABS.MAP_LSIT,
    title: "Map",
    icon: <MapIcon />,
    taskType: TaskTypeEnum.MAP,
  },
  // {
  //   id: TABS.CHART_LIST,
  //   title: "Country Chart",
  //   icon: <ChartIcon />,
  //   taskType: TaskTypeEnum.CHART,
  // }
];


export default function App() {
  const [tab, setTab] = useState(TABS.MAP_LSIT);
  const [selectedTaskId, setSelectedTaskId] = useState("");
  const [tasks, setTasks] = useState<Task[]>([])

  useReplaceSession();

  const sessionId = window.localStorage.getItem(SESSION_ID_KEY);
  const username = window.localStorage.getItem(USERNAME_KEY);

  const selectedTaskType = useMemo(() => {
    if ([TABS.MAP_LSIT, TABS.MAP_DETAILS].includes(tab)) {
      return TaskTypeEnum.MAP
    }
    return TaskTypeEnum.CHART
  }, [tab])

  const onLogout = () => {
    logout().finally(() => {
      window.localStorage.removeItem(SESSION_ID_KEY);
      window.localStorage.removeItem(USERNAME_KEY);
      window.location.reload();
    })
  }

  const onNewClick = () => {
    if (selectedTaskType === TaskTypeEnum.MAP) {
      setSelectedTaskId("");
      setTab(TABS.MAP_DETAILS);
    } else if (selectedTaskType == TaskTypeEnum.CHART) {
      setSelectedTaskId("");
      setTab(TABS.CHART_DETAILS);
    }
  }

  const onTaskClick = (task: Task) => {
    setSelectedTaskId(task.id);
    if (selectedTaskType === TaskTypeEnum.MAP) {
      setTab(TABS.MAP_DETAILS);
    } else if (selectedTaskType == TaskTypeEnum.CHART) {
      setTab(TABS.CHART_DETAILS);
    }
  }

  const onNavigateToList = useCallback(() => {
    setTab(TABS.MAP_LSIT);
    window.scrollTo({ left: 0, top: 0 })
  }, [setTab])

  useEffect(() => {
    if (sessionId && [TABS.MAP_LSIT, TABS.CHART_LIST].includes(tab)) {
      console.log("SHould get task list");
      fetchTasks(selectedTaskType)
        .then(res => {
          if (res.tasks) {
            console.log({ res });
            setTasks(res.tasks);
          }
        })
        .catch(err => {
          console.log({ err });
        })

    }
  }, [tab, setTasks, selectedTaskType, sessionId])

  return (
    <Box sx={{ display: 'flex' }}>
      <AppBar position="fixed" sx={{ zIndex: (theme) => theme.zIndex.drawer + 1 }}>
        <Toolbar>
          <Typography variant="h6" noWrap component="div">
            OWID Importer Tool
          </Typography>
        </Toolbar>
      </AppBar>
      {sessionId ? (
        <>
          <Drawer
            variant="permanent"
            sx={{
              width: drawerWidth,
              flexShrink: 0,
              [`& .MuiDrawer-paper`]: { width: drawerWidth, boxSizing: 'border-box' },
            }}
          >
            <Toolbar />
            <Box sx={{ overflow: 'auto' }}>
              <List>
                {LIST_ITEMS.map(item => (
                  <ListItem key={item.id} disablePadding>
                    <ListItemButton onClick={() => setTab(item.id)} selected={item.taskType === selectedTaskType}>
                      <ListItemIcon>
                        {item.icon}
                      </ListItemIcon>
                      <ListItemText primary={item.title} />
                    </ListItemButton>
                  </ListItem>
                ))}
                <ListItem disablePadding>
                  <ListItemButton onClick={onLogout}>
                    <ListItemIcon>
                      <Logout sx={{ transform: "rotate(180deg)" }} />
                    </ListItemIcon>
                    <ListItemText primary={"Logout"} secondary={username} />
                  </ListItemButton>
                </ListItem>
              </List>
            </Box>
          </Drawer>
          <Box component="main" sx={{ flexGrow: 1, p: 3 }}>
            <Toolbar />
            {[TABS.MAP_LSIT, TABS.CHART_LIST].includes(tab) ? (
              <TaskList tasks={tasks} taskType={selectedTaskType} onNew={onNewClick} onTaskClick={onTaskClick} />
            ) : (
              <>
                {selectedTaskType == TaskTypeEnum.MAP ? (
                  <MapImporter taskId={selectedTaskId} onNavigateToList={onNavigateToList} />
                ) : (
                  <ChartImporter taskId={selectedTaskId} />
                )}
              </>
            )}
          </Box>
        </>
      ) : (
        <Box component="main" sx={{ flexGrow: 1, p: 3 }}>
          <Toolbar />
          <Login />
        </Box>
      )}
    </Box>
  );
}

