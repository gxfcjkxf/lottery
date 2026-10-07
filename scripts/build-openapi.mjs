import { readFile,writeFile } from "node:fs/promises";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { dirname,resolve } from "node:path";
import { composeDocument,documentedRoutes } from "./openapi-lib.mjs";
const root=resolve(dirname(fileURLToPath(import.meta.url)),"..");
const modules=await Promise.all(["identity","finance","lottery"].map(name=>import(new URL(`../docs/openapi/${name}.mjs`,import.meta.url))));
modules.push(await import("./openapi-withdrawal-orders.mjs"));
modules.push(await import("./openapi-withdrawal-reports.mjs"));
modules.push(await import("./openapi-workbench.mjs"));
modules.push(await import("./openapi-commission-policies.mjs"));
modules.push(await import("./openapi-commission-cycles.mjs"));
const result=spawnSync(process.env.LOTTERY_GO_BIN??"go",["run","-buildvcs=false","./cmd/route-inventory"],{cwd:resolve(root,"backend"),encoding:"utf8",env:{...process.env,CGO_ENABLED:"0"},maxBuffer:8*1024*1024});
if(result.error||result.status!==0)throw new Error(`Cannot inspect real routes: ${result.error?.message??result.stderr}`);
const routes=JSON.parse(result.stdout),doc=composeDocument(modules,routes),text=JSON.stringify(doc,null,2)+"\n",destination=resolve(root,"docs/openapi.json");
if(process.argv.includes("--check")){const current=await readFile(destination,"utf8");if(current!==text)throw new Error("docs/openapi.json is stale; run pnpm api:generate");console.log(`OpenAPI current: ${documentedRoutes(doc).length} registered operations, ${Object.keys(doc.components.schemas).length} schemas`)}else{await writeFile(destination,text);console.log(`Generated ${destination}: ${documentedRoutes(doc).length} registered operations`)}
