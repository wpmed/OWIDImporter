import { Button, Snackbar } from "@mui/material";
import { useCallback, useState } from "react";
import { copyText } from "../utils";
import ContentCopyIcon from '@mui/icons-material/ContentCopy';

export function CopyButton({ text }: { text: string }) {
  const [isCopied, setIsCopied] = useState(false);

  const onCopy = useCallback(() => {
    copyText(text);
    setIsCopied(true);
  }, [text, setIsCopied]);

  return (
    <>
      <Snackbar
        open={isCopied}
        autoHideDuration={2000}
        onClose={() => setIsCopied(false)}
        message="Copied succesfully"
      />
      <Button size="small" onClick={onCopy} ><ContentCopyIcon fontSize="small" /></Button>
    </>
  )
}
