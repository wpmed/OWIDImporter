import { Box, CircularProgress } from "@mui/material";

export function FieldLoading() {
  return (
    <Box sx={{ position: "absolute", right: "1%", top: "10%", height: "80%", paddingRight: 1, paddingLeft: 1, display: "flex", alignItems: "center" }}>
      <CircularProgress size={20} />
    </Box>
  )
}
