import LinkIcon from '@mui/icons-material/Link';
import { useLocation } from "react-router";
import { CopyButton } from "./CopyButton";

// Copies the public address of the current page (an import anyone can open read-only).
export function ShareLinkButton() {
  const { pathname } = useLocation();
  return (
    <CopyButton
      text={window.location.origin + pathname}
      label="Copy link"
      icon={<LinkIcon />}
      successMessage="Link copied"
    />
  )
}
