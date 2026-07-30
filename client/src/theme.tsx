import { alpha, createTheme } from '@mui/material/styles';

export const serifStack = "'Fraunces Variable', Georgia, 'Times New Roman', serif";
export const sansStack = "'IBM Plex Sans', 'Helvetica Neue', Arial, sans-serif";
export const monoStack = "'IBM Plex Mono', 'SFMono-Regular', Consolas, monospace";

export type StatusKind =
  | 'done'
  | 'failed'
  | 'cancelled'
  | 'processing'
  | 'retrying'
  | 'queued';

export type StatusPalette = Record<StatusKind, string>;

declare module '@mui/material/styles' {
  interface Palette {
    status: StatusPalette;
  }
  interface PaletteOptions {
    status?: StatusPalette;
  }
}

const ink = '#1C1B1A';
const paper = '#FAF7F2';
const hairline = 'rgba(28, 27, 26, 0.14)';

const theme = createTheme({
  palette: {
    mode: 'light',
    background: {
      default: paper,
      paper: '#FFFFFF',
    },
    text: {
      primary: ink,
      secondary: '#6B655C',
    },
    divider: hairline,
    primary: {
      main: '#1D3D63',
      light: '#3360A9',
      dark: '#122B47',
    },
    secondary: {
      main: '#B13507',
    },
    success: { main: '#2C8465' },
    error: { main: '#A82A2A' },
    warning: { main: '#B8860B' },
    info: { main: '#3360A9' },
    status: {
      done: '#2C8465',
      failed: '#A82A2A',
      cancelled: '#8A7F72',
      processing: '#3360A9',
      retrying: '#D97A00',
      queued: '#64748B',
    },
  },
  shape: {
    borderRadius: 6,
  },
  typography: {
    fontFamily: sansStack,
    h1: { fontFamily: serifStack, fontWeight: 600, letterSpacing: '-0.01em' },
    h2: { fontFamily: serifStack, fontWeight: 600, letterSpacing: '-0.01em' },
    h3: { fontFamily: serifStack, fontWeight: 600, letterSpacing: '-0.01em' },
    h4: { fontFamily: serifStack, fontWeight: 600, letterSpacing: '-0.01em' },
    h5: { fontWeight: 600 },
    h6: { fontWeight: 600 },
    overline: {
      fontWeight: 600,
      letterSpacing: '0.08em',
    },
    button: {
      textTransform: 'none',
      fontWeight: 600,
    },
  },
  components: {
    MuiCssBaseline: {
      styleOverrides: {
        body: {
          backgroundImage: `linear-gradient(to bottom, #F3EDE3, ${paper} 320px)`,
          backgroundRepeat: 'no-repeat',
        },
      },
    },
    MuiAppBar: {
      defaultProps: {
        elevation: 0,
        color: 'transparent',
      },
      styleOverrides: {
        root: ({ theme }) => ({
          backgroundColor: alpha(paper, 0.85),
          backdropFilter: 'blur(8px)',
          color: theme.palette.text.primary,
          borderBottom: `1px solid ${theme.palette.divider}`,
        }),
      },
    },
    MuiDrawer: {
      styleOverrides: {
        paper: ({ theme }) => ({
          backgroundColor: 'transparent',
          borderRight: `1px solid ${theme.palette.divider}`,
        }),
      },
    },
    MuiListItemButton: {
      styleOverrides: {
        root: ({ theme }) => ({
          borderRadius: theme.shape.borderRadius,
          margin: theme.spacing(0.25, 1),
          '&.Mui-selected': {
            backgroundColor: alpha(theme.palette.primary.main, 0.08),
            boxShadow: `inset 3px 0 0 ${theme.palette.primary.main}`,
            '&:hover': {
              backgroundColor: alpha(theme.palette.primary.main, 0.12),
            },
          },
        }),
      },
    },
    MuiListItemIcon: {
      styleOverrides: {
        root: {
          minWidth: 40,
        },
      },
    },
    MuiCard: {
      defaultProps: {
        elevation: 0,
      },
      styleOverrides: {
        root: ({ theme }) => ({
          border: `1px solid ${theme.palette.divider}`,
          transition: theme.transitions.create(['box-shadow', 'border-color'], {
            duration: theme.transitions.duration.short,
          }),
          '&:hover': {
            borderColor: alpha(ink, 0.28),
            boxShadow: `0 2px 12px ${alpha(ink, 0.08)}`,
          },
        }),
      },
    },
    MuiButton: {
      defaultProps: {
        disableElevation: true,
      },
    },
    MuiChip: {
      styleOverrides: {
        root: {
          fontWeight: 500,
        },
      },
    },
    MuiOutlinedInput: {
      styleOverrides: {
        root: ({ theme }) => ({
          backgroundColor: theme.palette.background.paper,
        }),
      },
    },
    MuiAccordion: {
      defaultProps: {
        disableGutters: true,
        elevation: 0,
      },
      styleOverrides: {
        root: ({ theme }) => ({
          border: `1px solid ${theme.palette.divider}`,
          borderRadius: theme.shape.borderRadius,
          '&::before': {
            display: 'none',
          },
        }),
      },
    },
    MuiDialog: {
      styleOverrides: {
        paper: {
          borderRadius: 8,
        },
      },
    },
    MuiTooltip: {
      styleOverrides: {
        tooltip: {
          backgroundColor: ink,
          fontSize: 12,
        },
      },
    },
  },
});

export default theme;
