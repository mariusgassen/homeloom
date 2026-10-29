import Markdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { photoUrl } from "../notes";

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

// A note's own photos, which the API serves, are shown; any other image still shows only its alt text.
const noteComponents: Components = {
  ...components,
  img: ({ src, alt }) => (typeof src === "string" && photoUrl.test(src) ? <img className="note-photo" src={src} alt={alt ?? ""} loading="lazy" /> : <>{alt}</>),
};

// Markdown from the assistant, as it streams in: half-finished syntax renders as text until it closes.
// With photos, it is a note's text and shows the note's photos.
export function MarkdownText({ text, photos = false }: { text: string; photos?: boolean }) {
  return (
    <Markdown remarkPlugins={[remarkGfm]} components={photos ? noteComponents : components}>
      {text}
    </Markdown>
  );
}
