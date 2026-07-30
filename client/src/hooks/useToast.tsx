/* eslint-disable react-refresh/only-export-components */
import { Alert, AlertColor, Snackbar } from "@mui/material";
import { createContext, ReactNode, useCallback, useContext, useMemo, useState } from "react";

interface ToastState {
  message: string
  severity: AlertColor
}

interface ToastContextValue {
  showToast: (message: string, severity?: AlertColor) => void
}

const ToastContext = createContext<ToastContextValue | null>(null);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toast, setToast] = useState<ToastState | null>(null);
  const [open, setOpen] = useState(false);

  const showToast = useCallback((message: string, severity: AlertColor = "success") => {
    setToast({ message, severity });
    setOpen(true);
  }, []);

  const value = useMemo(() => ({ showToast }), [showToast]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      <Snackbar
        open={open}
        autoHideDuration={4000}
        onClose={() => setOpen(false)}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert
          variant="filled"
          severity={toast?.severity ?? "success"}
          onClose={() => setOpen(false)}
          sx={{ minWidth: 280 }}
        >
          {toast?.message}
        </Alert>
      </Snackbar>
    </ToastContext.Provider>
  )
}

export function useToast() {
  const context = useContext(ToastContext);
  if (!context) {
    throw new Error("useToast must be used within a ToastProvider");
  }
  return context;
}
