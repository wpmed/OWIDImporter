import { Button } from "@mui/material";
import { ReactNode, useCallback } from "react";
import { copyText } from "../utils";
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { useToast } from "../hooks/useToast";

interface CopyButtonProps {
  text: string
  // With a label the button is an outlined text button; without, an icon button
  label?: string
  icon?: ReactNode
  successMessage?: string
}

export function CopyButton({ text, label, icon, successMessage = "Copied successfully" }: CopyButtonProps) {
  const { showToast } = useToast();

  const onCopy = useCallback(() => {
    copyText(text);
    showToast(successMessage);
  }, [text, successMessage, showToast]);

  if (label) {
    return (
      <Button variant="outlined" onClick={onCopy} startIcon={icon ?? <ContentCopyIcon fontSize="small" />}>
        {label}
      </Button>
    )
  }

  return (
    <Button size="small" onClick={onCopy}><ContentCopyIcon fontSize="small" /></Button>
  )
}
