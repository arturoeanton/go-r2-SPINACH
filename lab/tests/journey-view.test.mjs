import test from 'node:test';
import assert from 'node:assert/strict';
import {traceSteps} from '../internal/app/web/journey-view.js';

test('step cards show the actual model, evidence and time and escape context data',()=>{
  const run={mode:'ai',measureTime:true,traces:[{
    title:'Ejecutar consulta',durationMs:1250,actor:'OpenAI',
    action:{type:'query',query:'SELECT ?name WHERE {?s ?p ?name}'},
    generation:{model:'gpt-5',provider:'openai',input:{question:'<script>untrusted</script>',entityDirectory:[],initialSubgraph:[],observations:[]}},
    observation:{variables:['name'],rows:[{name:{label:'Sofía Vega'}}]},
  }]};
  const html=traceSteps(run,{subgraphs:[]},[]);
  assert.match(html,/1\.25 s/);
  assert.match(html,/gpt-5/);
  assert.match(html,/Sofía Vega/);
  assert.match(html,/Inspect generator context/);
  assert.ok(!html.includes('<script>'));
  assert.match(html,/&lt;script&gt;/);
  run.measureTime=false;
  const untimed=traceSteps(run,{subgraphs:[]},[]);
  assert.match(untimed,/Timing off/);
  assert.ok(!untimed.includes('1.25 s'));
});
