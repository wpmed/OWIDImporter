import { Box, Paper } from "@mui/material";
import { CopyButton } from "../CopyButton";
import { monoStack } from "../../theme";

export function CodeBlock({ code }: { code: string }) {
  return (
    <Paper variant="outlined" sx={{ position: 'relative', bgcolor: '#F3EFE7' }}>
      <Box sx={{ position: 'absolute', top: 4, right: 4 }}>
        <CopyButton text={code} />
      </Box>
      <Box
        component="pre"
        sx={{
          m: 0,
          p: 2,
          fontFamily: monoStack,
          fontSize: 13,
          lineHeight: 1.6,
          overflow: 'auto',
        }}
      >
        {code}
      </Box>
    </Paper>
  )
}
