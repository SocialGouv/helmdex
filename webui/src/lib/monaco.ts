// Bundle monaco locally instead of @monaco-editor/react's default CDN load:
// the desktop app must work offline.
import * as monaco from "monaco-editor";
// Paths go through monaco's exports map ("./*" -> "./esm/vs/*.js").
import editorWorker from "monaco-editor/editor/editor.worker?worker";
import jsonWorker from "monaco-editor/language/json/json.worker?worker";
import { loader } from "@monaco-editor/react";

self.MonacoEnvironment = {
  getWorker(_workerId: string, label: string) {
    if (label === "json") return new jsonWorker();
    return new editorWorker();
  },
};

loader.config({ monaco });
