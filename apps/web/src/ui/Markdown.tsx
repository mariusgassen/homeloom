import Markdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

// Raw HTML is never rendered and unsafe link protocols are dropped (react-markdown's defaults).
// Images show only their alt text, so an answer can't make the app load a URL by itself.
const components: Components = {
  a: ({ href, title, children }) => (
    <a href={href} title={title} target="_blank" rel="noopener noreferrer">
      {children}
    </a>
  ),
  img: ({ alt }) => <>{alt}</>,
  table: ({ children }) => (
    <div className="md-table">
      <table>{children}</table>
    </div>
  ),
};

// Markdown from the assistant, as it streams in: half-finished syntax renders as text until it closes.
export function MarkdownText({ text }: { text: string }) {
  return (
    <Markdown remarkPlugins={[remarkGfm]} components={components}>
      {text}
    </Markdown>
  );
}
