import React from "react";
import ReactDOM from "react-dom/client";
import { App } from "./App";
import "./tokeny.css";

// The root exists in index.html; the assertion is the only thing that would
// ever tell you if someone renamed it.
const koren = document.getElementById("root");
if (!koren) throw new Error("в index.html нет #root");

ReactDOM.createRoot(koren).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
