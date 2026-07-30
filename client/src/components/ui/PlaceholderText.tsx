import { Box } from "@mui/material";
import { monoStack } from "../../theme";

// Renders help copy, wrapping $PLACEHOLDER tokens in inline code styling.
export function PlaceholderText({ text }: { text: string }) {
  const parts = text.split(/(\$[A-Z_]+)/g);
  return (
    <>
      {parts.map((part, index) =>
        /^\$[A-Z_]+$/.test(part) ? (
          <Box
            key={index}
            component="code"
            sx={{
              fontFamily: monoStack,
              fontSize: '0.85em',
              bgcolor: 'rgba(28, 27, 26, 0.06)',
              px: 0.5,
              borderRadius: 0.5,
            }}
          >
            {part}
          </Box>
        ) : (
          part
        )
      )}
    </>
  )
}
