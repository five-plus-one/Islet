import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { TooltipProvider } from "@fiveplusone/ui";
import "@fiveplusone/ui/styles.css";
import "@fiveplusone/ui/app-surfaces.css";
import "./style.css";
import { App } from "./App";
document.documentElement.dataset.product = "brand";
document.documentElement.dataset.theme =
  localStorage.getItem("islet-theme") ||
  (matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
createRoot(document.getElementById("app")!).render(
  <BrowserRouter>
    <TooltipProvider>
      <App />
    </TooltipProvider>
  </BrowserRouter>,
);
