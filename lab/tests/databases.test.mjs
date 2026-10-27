import test from 'node:test';
import assert from 'node:assert/strict';
import {deleteDatabaseForm} from '../internal/app/web/databases.js';

function setup(t, fail=false){
  const calls=[];
  const state={g:{id:'upload-one',sqliteSource:'source-one',title:'SQLite test'},dirty:true,graphs:[]};
  state.graphs=[state.g];
  let submit;
  const previous=globalThis.document;
  globalThis.document={querySelector:()=>({classList:{remove(){},add(){}}})};
  t.after(()=>{globalThis.document=previous});
  const deps={state,esc:s=>s,form:(_title,_body,save)=>{submit=save},
    stopRun:()=>calls.push('stop'),
    api:async(path,_body,method)=>{calls.push([method,path]);if(fail)throw Error('disk error')},
    loadGraphs:async()=>{calls.push('reload')},toast:()=>calls.push('success')};
  deleteDatabaseForm(deps);
  return {state,calls,submit};
}

test('opening or cancelling deletion does not stop a journey or send a delete',t=>{
  const {calls}=setup(t);
  assert.deepEqual(calls,[]);
});

test('confirmation stops active work and deletes the graph named in the dialog',async t=>{
  const {state,calls,submit}=setup(t);
  state.g={id:'another-graph'};
  await submit();
  assert.deepEqual(calls,['stop',['DELETE','/graphs/upload-one'],'reload','success']);
  assert.equal(state.dirty,false);
});

test('failed deletion preserves unsaved changes and does not report success',async t=>{
  const {state,calls,submit}=setup(t,true);
  await assert.rejects(submit,/disk error/);
  assert.equal(state.dirty,true);
  assert.deepEqual(calls,['stop',['DELETE','/graphs/upload-one']]);
});
