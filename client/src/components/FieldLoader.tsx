import { CircularProgress, InputAdornment } from "@mui/material";

// Spinner for a TextField's endAdornment, so the input keeps it vertically centred
export function FieldLoading() {
  return (
    <InputAdornment position="end">
      <CircularProgress size={18} />
    </InputAdornment>
  )
}
