import { Button } from "@mui/material";
import { useCallback } from "react";
import { copyText } from "../utils";
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { useToast } from "../hooks/useToast";

export function CopyButton({ text }: { text: string }) {
  const { showToast } = useToast();

  const onCopy = useCallback(() => {
    copyText(text);
    showToast("Copied successfully");
  }, [text, showToast]);

  return (
    <Button size="small" onClick={onCopy}><ContentCopyIcon fontSize="small" /></Button>
  )
}
