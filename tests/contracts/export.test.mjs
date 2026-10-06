import {test} from "node:test";
import assert from "node:assert/strict";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import {composeDocument} from "../../scripts/openapi-lib.mjs";
import {operations,schemas} from "../../docs/openapi/finance.mjs";

const exportOperations=operations.filter(operation=>operation.path.endsWith("/export"));
const routes=exportOperations.map(({method,path})=>({method,path}));
const document=()=>composeDocument([{schemas,operations:exportOperations}],routes);

test("implemented CSV exports declare bounded binary responses, headers, filters, and permissions",()=>{
  const doc=document();
  for(const kind of ["betting","ledger"]){
    const path=`/api/v1/admin/reports/${kind}/export`;
    const operation=doc.paths[path].get;
    assert.deepEqual(operation.security,[{adminBearer:[]},{adminCookie:[]}]);
    assert.deepEqual(operation["x-permissions"],[`report_${kind}.view.brand`,`report_${kind}.export.brand`,`report_${kind}.view.platform`,`report_${kind}.export.platform`]);
    assert.match(operation.description,new RegExp(`\\(report_${kind}\\.view\\.brand OR report_${kind}\\.view\\.platform\\) AND \\(report_${kind}\\.export\\.brand OR report_${kind}\\.export\\.platform\\)`));
    assert.match(operation.description,/Each brand-scoped grant must match the selected brand membership/);
    assert.deepEqual(operation.parameters.filter(parameter=>parameter.in==="query").map(parameter=>parameter.name),["from","to","group_by",...(kind==="betting"?["game_id"]:[]),"member_id"]);
    assert.ok(!operation.parameters.some(parameter=>["limit","offset"].includes(parameter.name)));
    const response=operation.responses["200"];
    assert.deepEqual(Object.keys(response.content),["text/csv"]);
    assert.deepEqual(response.content["text/csv"].schema,{type:"string",format:"binary",description:response.content["text/csv"].schema.description});
    assert.match(response.content["text/csv"].schema.description,/BOM and LF/);
    for(const header of ["Content-Type","Content-Length","Content-Disposition","Cache-Control","X-Content-Type-Options","X-Report-Brand-ID","X-Report-Kind","X-Report-Snapshot-At","X-Report-Group-Count","X-Report-SHA256","X-Report-Format-Version","X-Report-Audit-ID"]){
      assert.ok(response.headers[header],`${kind} missing ${header}`);
    }
    assert.ok(operation.responses["413"].content["application/json"].schema.$ref.endsWith("/ErrorResponse"));
    assert.match(operation.description,/limit and offset pagination are not accepted/);
    assert.match(response.description,/one SQL statement snapshot/);
    assert.match(response.description,/audit record is committed before any response bytes/);
  }
  assert.deepEqual(doc.paths["/api/v1/admin/reports/betting/export"].get.parameters.find(parameter=>parameter.name==="group_by").schema.enum,["day","game","member"]);
  assert.deepEqual(doc.paths["/api/v1/admin/reports/ledger/export"].get.parameters.find(parameter=>parameter.name==="group_by").schema.enum,["day","entry_type"]);
});

test("builder preserves JSON envelopes and rejects missing or conflicting success declarations",()=>{
  const base={method:"GET",path:"/api/v1/admin/example",operationId:"example",summary:"Example",tag:"reporting",auth:"admin"};
  const binary={...base,operationId:"binaryExample",path:"/api/v1/admin/binary",successContent:{"text/csv":{schema:{type:"string",format:"binary"}}}};
  const json={...base,data:{$ref:"#/components/schemas/EmptyObject"}};
  const doc=composeDocument([{schemas:{},operations:[json,binary]}],[{method:"GET",path:json.path},{method:"GET",path:binary.path}]);
  assert.deepEqual(doc.paths[json.path].get.responses["200"].content["application/json"].schema,{
    type:"object",required:["success","data","request_id"],additionalProperties:false,
    properties:{success:{const:true},data:{$ref:"#/components/schemas/EmptyObject"},request_id:{type:"string"}},
  });
  assert.deepEqual(doc.paths[binary.path].get.responses["200"].content,binary.successContent);
  assert.throws(()=>composeDocument([{schemas:{},operations:[{...base,operationId:"missing"}]}],[{method:"GET",path:base.path}]),/success declaration/);
  assert.throws(()=>composeDocument([{schemas:{},operations:[{...binary,data:{$ref:"#/components/schemas/EmptyObject"}}]}],[{method:"GET",path:binary.path}]),/success declaration/);
  assert.throws(()=>composeDocument([{schemas:{},operations:[{...binary,data:undefined}]}],[{method:"GET",path:binary.path}]),/success declaration/);
  assert.throws(()=>composeDocument([{schemas:{},operations:[{...base,operationId:"missingContentSchema",successContent:{"text/csv":{description:"Missing schema"}}}]}],[{method:"GET",path:base.path}]),/Invalid successContent/);
  assert.throws(()=>composeDocument([{schemas:{},operations:[{...base,operationId:"emptyContent",successContent:{}}]}],[{method:"GET",path:base.path}]),/Invalid successContent/);
});

test("every CSV export response media schema compiles, including binary content",()=>{
  const doc=document();
  const ajv=new Ajv2020({strict:false,allErrors:true});
  addFormats(ajv);
  ajv.addFormat("int64",{type:"number",validate:Number.isInteger});
  for(const item of Object.values(doc.paths))for(const operation of Object.values(item)){
    for(const response of Object.values(operation.responses))for(const media of Object.values(response.content??{})){
      if(media.schema)ajv.compile({...media.schema,components:doc.components});
    }
  }
});
