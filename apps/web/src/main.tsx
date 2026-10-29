import "@fontsource-variable/bricolage-grotesque";
import "./styles/tokens.css";
import "./styles/components.css";
import "./styles/app.css";
import "./i18n";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router";
import { App } from "./App";
import { Crashed } from "./screens/Welcome";
import { SessionProvider } from "./session";
import { trackViewport } from "./viewport";

const root = document.getElementById("root");
if (!root) throw new Error("#root missing from index.html");
trackViewport();

createRoot(root).render(
  <StrictMode>
    <Crashed>
      <BrowserRouter>
        <SessionProvider>
          <App />
        </SessionProvider>
      </BrowserRouter>
    </Crashed>
  </StrictMode>,
);
